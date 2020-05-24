package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
	"github.com/rs/zerolog"
	"github.com/vmihailenco/msgpack"
)

// BufferSize sets a maximum buffer size for channels
const BufferSize = 2048

// Manager representation for multiple shards
type Manager struct {
	Token         string
	Identity      string
	Configuration managerConfiguration
	log           zerolog.Logger

	// eventChannel is a channel that all dispatched events are sent to
	eventChannel chan Event

	// produceChannel is a channel with all streamevents to be piped to
	// STAN/NATS. This is seperated from eventChannel to allow for direct
	// piping to STAN/NATS without being processed such as CONNECT events.
	produceChannel chan StreamEvent

	// Sessions contains a list of all the sessions present
	Sessions []*Session

	// The http client used for REST requests
	Client *http.Client

	// The user agent used for REST APIs
	UserAgent string

	// Stores
	GatewayResponse *GatewayBotResponse
	ShardCount      int

	// List of unavailable guilds from READY events
	Unavailables map[string]bool

	// Presence that each session will be given
	Presence UpdateStatusData
}

// ManagerConfiguration represents all configurable elements
type managerConfiguration struct {
	redisOptions *redis.Options
	redisClient  *redis.Client
	natsClient   *nats.Conn
	stanClient   stan.Conn

	// Manual sharding
	Autoshard  bool `json:"autoshard"`
	ShardCount int  `json:"shard_count"`

	// Authentication for redis client
	RedisAddress  string `json:"redis_address"`
	RedisPassword string `json:"redis_password"`
	RedisDatabase int    `json:"redis_database"`

	// RedisPrefix represents what keys will be prepended with when keys are constructed
	RedisPrefix string `json:"redis_prefix"`

	// Configuration for NATS
	NatsAddress string `json:"nats_address"`
	NatsChannel string `json:"nats_channel"`
	ClusterID   string `json:"nats_cluster"`
	ClientID    string `json:"nats_client"`

	// IgnoredEvents shows events that will be completely ignored if they are dispatched
	IgnoredEvents []string `json:"ignored"`

	// EventBlacklist shows events that will have its information cached but will not be
	// relayed to consumers.
	ProducerBlacklist []string `json:"blacklist"`
}

// NewManager creates all the shards and shenanigans
func NewManager(token string, identity string, configuration managerConfiguration, log zerolog.Logger, presence UpdateStatusData) (m *Manager) {
	m = &Manager{
		Token:          token,
		Identity:       identity,
		Configuration:  configuration,
		Client:         &http.Client{Timeout: (20 * time.Second)},
		UserAgent:      "DiscordBot (https://github.com/TheRockettek/Sandwich-Producer, v" + VERSION + ")",
		eventChannel:   make(chan Event, BufferSize),
		produceChannel: make(chan StreamEvent, BufferSize),
		Unavailables:   make(map[string]bool),
		log:            log,
		Presence:       presence,
	}
	m.Configuration.redisClient = redis.NewClient(configuration.redisOptions)

	return
}

// ClearCache removes keys from redis. There is definitely a more inteligent way of doing this.
func (m *Manager) ClearCache() (err error) {
	var keys []string
	iter := m.Configuration.redisClient.Scan(
		ctx,
		0,
		fmt.Sprintf("%s:*", m.Configuration.RedisPrefix),
		0,
	).Iterator()

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if len(keys) > 0 {
		if err = m.Configuration.redisClient.Del(
			ctx,
			keys...,
		).Err(); err != nil {
			m.log.Error().Err(err).Msg("failed to remove keys")
		}
	}

	m.log.Info().Int("count", len(keys)).Msg("removed keys")
	return
}

// Open starts up the manager and will retrieve appropriate information and start shards
func (m *Manager) Open() (err error) {
	gr, err := m.Gateway()
	if err != nil {
		return
	}

	m.GatewayResponse = gr

	m.log.Info().Str("gateway", m.GatewayResponse.URL).Int("shards", m.GatewayResponse.Shards).Int("remaining", m.GatewayResponse.SessionLimit.Remaining).Send()

	// We will use the recommended shard count if autoshard is enabled or the specified shard count is too small
	if m.Configuration.Autoshard == true || m.Configuration.ShardCount < m.GatewayResponse.Shards {
		m.ShardCount = m.GatewayResponse.Shards
	} else {
		m.ShardCount = m.Configuration.ShardCount
	}

	if m.ShardCount > m.GatewayResponse.SessionLimit.Remaining {
		m.log.Fatal().Int("shards", m.ShardCount).Int("remaining", m.GatewayResponse.SessionLimit.Remaining).Msg("not enough sessions remaining")
	}

	m.log.Info().Int("shards", m.ShardCount).Bool("autosharded", m.Configuration.Autoshard).Msg("creating sessions")
	m.Sessions = make([]*Session, m.ShardCount)

	for shardID := 0; shardID < m.ShardCount; shardID++ {
		m.Sessions[shardID] = NewSession(m.Token, shardID, m.ShardCount, m.eventChannel, m.log, m.GatewayResponse.URL)
		m.Sessions[shardID].Presence = m.Presence
	}

	go m.ForwardEvents()
	go m.ForwardProduce()

	for shardID, session := range m.Sessions {
		m.log.Info().Int("shard", shardID).Msg("starting session")
		err = session.Open()
		if err != nil {
			return
		}
	}

	return
}

// Gateway returns the gateway url and recommended shard count
func (m *Manager) Gateway() (st *GatewayBotResponse, err error) {
	var req *http.Request
	var response []byte

	req, err = http.NewRequest("GET", EndpointGatewayBot, nil)
	req.Header.Set("authorization", "Bot "+m.Token)
	req.Header.Set("User-Agent", m.UserAgent)

	resp, err := m.Client.Do(req)
	if err != nil {
		return
	}

	defer func() {
		resp.Body.Close()
	}()

	response, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	switch resp.StatusCode {
	case 429:
		rl := TooManyRequests{}
		err = json.Unmarshal(response, &rl)
		if err != nil {
			m.log.Error().Err(err).Msg("rate limit unmarshal error")
			return
		}

		m.log.Warn().Dur("retry_after", rl.RetryAfter).Msg("gateway request was ratelimited")
		time.Sleep(rl.RetryAfter * time.Millisecond)
		return m.Gateway()
	case http.StatusUnauthorized:
		err = ErrInvalidToken
		return
	}

	err = json.Unmarshal(response, &st)
	if err != nil {
		return
	}

	return
}

// OnEvent will take the origional event data and then handle any caching information
// along with modifying the origional structure to a more suitable one for streaming.
func (m *Manager) OnEvent(e Event) (ok bool, se StreamEvent) {
	var data StreamEvent
	var ma func(*Manager, Event) (bool, StreamEvent)

	if ma, ok = marshalers[e.Type]; ok {
		ok, data = ma(m, e)
		if ok {
			se = data
		} else {
			m.log.Debug().Str("type", e.Type).Msg("marshaler reported not ok")
		}
	} else {
		m.log.Warn().Str("type", e.Type).Msg("no available marshaler")
	}

	// Marshaler may override this function (such as in GUILD_CREATE) so only change it
	// if it is empty!
	if se.Type == "" {
		se.Type = e.Type
	}

	return
}

// ForwardEvents is the global cache handler, once it has finished handling events,
// if it decides to sent it on to consumers, it should send it to the produce channel.
func (m *Manager) ForwardEvents() {
	var ok bool
	var se StreamEvent
	for e := range m.eventChannel {
		if belongsToList(m.Configuration.IgnoredEvents, e.Type) {
			m.log.Debug().Str("type", e.Type).Msg("event blacklisted")
			continue
		}

		ok, se = m.OnEvent(e)

		if ok && !belongsToList(m.Configuration.ProducerBlacklist, e.Type) {
			m.produceChannel <- se
		} else {
			m.log.Debug().Str("type", e.Type).Msg("ignoring event")
		}
	}
}

// ForwardProduce simply just routes messages it receives and will publish it to
// NATS/STAN
func (m *Manager) ForwardProduce() {
	var e StreamEvent
	var err error
	var ep []byte

	m.Configuration.natsClient, err = nats.Connect(m.Configuration.NatsAddress)
	if err != nil {
		m.log.Panic().Err(err).Send()
	}
	m.Configuration.stanClient, err = stan.Connect(m.Configuration.ClusterID,
		m.Configuration.ClientID, stan.NatsConn(m.Configuration.natsClient))
	if err != nil {
		m.log.Panic().Err(err).Send()
	}

	for e = range m.produceChannel {

		ep, err = msgpack.Marshal(e)
		if err != nil {
			m.log.Warn().Err(err).Msg("failed to marshal stream event")
			continue
		}
		err = m.Configuration.stanClient.Publish(m.Configuration.NatsChannel, ep)
		if err != nil {
			m.log.Warn().Err(err).Msg("failed to publish stream event")
			continue
		}
	}
}

// Close gracefully closes sessions and ensures all messages are sent before closing
func (m *Manager) Close() {
	m.log.Info().Msg("Closing sessions...")
	for _, s := range m.Sessions {
		s.Close()
	}

	// Allow time for late dispatchers
	time.Sleep(time.Second)

	for len(m.eventChannel) > 0 && len(m.produceChannel) > 0 {
		m.log.Info().Int("event", len(m.eventChannel)).Int("produce", len(m.produceChannel)).Msg("Waiting for channels...")
		time.Sleep(time.Second)
	}
	close(m.eventChannel)
	close(m.produceChannel)
}
