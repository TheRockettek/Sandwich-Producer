package main

import (
	"fmt"
	"io/ioutil"
	"math"
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
	Sessions map[int]*Session

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

// features defines different settings for specific things that the
// gateway can do
type features struct {
	CacheMembers bool `json:"cache_members"`
	// TODO: IgnoreBots: Will ignore bots in MESSAGE_* events
	// TODO: CheckPrefix: MESSAGE_CREATE will not pass message if message does not have the guild prefix set.
	// TODO: StoreMutuals: Add checks to the current mutual setup
}

// ManagerConfiguration represents all configurable elements
type managerConfiguration struct {
	redisOptions *redis.Options
	redisClient  *redis.Client
	natsClient   *nats.Conn
	stanClient   stan.Conn

	// States
	Features features `json:"features" msgpack:"features"`

	// Manual sharding
	Autoshard  bool `json:"autoshard" msgpack:"autoshard"`
	ShardCount int  `json:"shard_count" msgpack:"shard_count"`

	// Authentication for redis client
	RedisAddress  string `json:"redis_address" msgpack:"redis_address"`
	RedisPassword string `json:"redis_password" msgpack:"redis_password"`
	RedisDatabase int    `json:"redis_database" msgpack:"redis_database"`

	// RedisPrefix represents what keys will be prepended with when keys are constructed
	RedisPrefix string `json:"redis_prefix" msgpack:"redis_prefix"`

	// Configuration for NATS
	NatsAddress string `json:"nats_address" msgpack:"nats_address"`
	NatsChannel string `json:"nats_channel" msgpack:"nats_channel"`
	ClusterID   string `json:"nats_cluster" msgpack:"nats_cluster"`
	ClientID    string `json:"nats_client" msgpack:"nats_client"`

	// IgnoredEvents shows events that will be completely ignored if they are dispatched
	IgnoredEvents []string `json:"ignored" msgpack:"ignored"`

	// EventBlacklist shows events that will have its information cached but will not be
	// relayed to consumers.
	ProducerBlacklist []string `json:"blacklist" msgpack:"blacklist"`
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
	deleted := int64(0)
	iter := m.Configuration.redisClient.Scan(
		ctx,
		0,
		fmt.Sprintf("%s:*", m.Configuration.RedisPrefix),
		8,
	).Iterator()

	m.log.Info().Msg("deleting keys...")
	now := time.Now()

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())

		if len(keys) > 256 {
			_deleted, err := m.Configuration.redisClient.Del(
				ctx,
				keys...,
			).Result()
			deleted += _deleted
			if err != nil {
				m.log.Error().Err(err).Msg("failed to remove keys")
			}
			keys = []string{}
		}

		_now := time.Now()
		if _now.Sub(now) > time.Second {
			m.log.Info().Msgf("deleted %d keys so far", deleted)
			now = _now
		}
	}

	if len(keys) > 0 {
		_deleted, err := m.Configuration.redisClient.Del(
			ctx,
			keys...,
		).Result()
		deleted += _deleted
		if err != nil {
			m.log.Error().Err(err).Msg("failed to remove keys")
		}
	}

	m.log.Info().Int64("deleted", deleted).Int("total", len(keys)).Msg("removed keys")
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
	if m.Configuration.Autoshard == true || m.Configuration.ShardCount < int(math.Ceil(float64(m.GatewayResponse.Shards)/2.5)) {
		m.ShardCount = m.GatewayResponse.Shards
	} else {
		m.ShardCount = m.Configuration.ShardCount
	}

	if m.ShardCount > m.GatewayResponse.SessionLimit.Remaining {
		m.log.Fatal().Int("shards", m.ShardCount).Int("remaining", m.GatewayResponse.SessionLimit.Remaining).Msg("not enough sessions remaining")
	}

	m.log.Info().Int("shards", m.ShardCount).Bool("autosharded", m.Configuration.Autoshard).Msg("creating sessions")
	m.Sessions = make(map[int]*Session)

	go m.ForwardEvents()
	go m.ForwardProduce()

	for shardID := 0; shardID < m.ShardCount; shardID++ {
		m.log.Info().Int("shard", shardID).Msg("starting session")
		session := NewSession(m.Token, shardID, m.ShardCount, m.eventChannel, &m.log, m.GatewayResponse.URL)
		session.Presence = m.Presence
		m.Sessions[shardID] = session
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
		err = jsoniter.Unmarshal(response, &rl)
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

	err = jsoniter.Unmarshal(response, &st)
	if err != nil {
		return
	}

	return
}

// OnEvent will take the origional event data and then handle any caching information
// along with modifying the origional structure to a more suitable one for streaming.
func (m *Manager) OnEvent(e Event) (ok bool, se StreamEvent) {
	var data StreamEvent
	var ma func(*Manager, Event) (bool, StreamEvent, error)
	var err error

	if ma, ok = marshalers[e.Type]; ok {
		ok, data, err = ma(m, e)
		if ok {
			se = data
		}
	} else {
		m.log.Warn().Str("type", e.Type).Msg("no available marshaler")
	}

	// Marshaler may override this function (such as in GUILD_CREATE) so only change it
	// if it is empty!
	if se.Type == "" {
		se.Type = e.Type
	}

	if err != nil {
		m.log.Warn().Err(err).Msgf("Failed to handle %s due to error", se.Type)
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
			continue
		}

		ok, se = m.OnEvent(e)

		if ok && !belongsToList(m.Configuration.ProducerBlacklist, e.Type) {
			m.produceChannel <- se
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

func (m *Manager) getGuild(id string) (g MarshalGuild, err error) {
	guildData, err := m.Configuration.redisClient.HGet(
		ctx,
		fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
		id,
	).Result()
	if err != nil {
		return
	}

	g = MarshalGuild{}
	if err = g.From([]byte(guildData)); err != nil {
		return
	}

	return
}

func (m *Manager) getChannel(id string) (c Channel, err error) {
	channelData, err := m.Configuration.redisClient.HGet(
		ctx,
		fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
		id,
	).Result()
	if err != nil {
		return
	}

	c = Channel{}
	if err = msgpack.Unmarshal([]byte(channelData), &c); err != nil {
	}

	return
}

func (m *Manager) getUser(userID string) (u User, err error) {
	userData, err := m.Configuration.redisClient.HGet(
		ctx,
		fmt.Sprintf("%s:user", m.Configuration.RedisPrefix),
		userID,
	).Result()
	if err != nil {
		return
	}

	u = User{}
	if err = msgpack.Unmarshal([]byte(userData), &u); err != nil {
		return
	}

	u.Mutual = MutualGuilds{}
	err = u.FetchMutual(m)

	return
}

func (m *Manager) getMember(guildID string, userID string) (me Member, err error) {
	memberData, err := m.Configuration.redisClient.HGet(
		ctx,
		fmt.Sprintf("%s:guild:%s:members", m.Configuration.RedisPrefix, guildID),
		userID,
	).Result()
	println(fmt.Sprintf("%s:guild:%s:members", m.Configuration.RedisPrefix, guildID), userID, err.Error())
	if err != nil {
		return
	}

	err = me.From([]byte(memberData), m)
	if err != nil {
		return
	}

	u, err := m.getUser(userID)
	me.User = &u

	return
}

func (m *Manager) getRole(guildID string, roleID string) (r Role, err error) {
	roleData, err := m.Configuration.redisClient.HGet(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guildID),
		roleID,
	).Result()
	if err != nil {
		return
	}

	r = Role{}
	if err = msgpack.Unmarshal([]byte(roleData), &r); err != nil {
		return
	}

	return
}

func (m *Manager) getEmoji(id string) (e Emoji, err error) {
	emojiData, err := m.Configuration.redisClient.HGet(
		ctx,
		fmt.Sprintf("%s:emojis", m.Configuration.RedisPrefix),
		id,
	).Result()
	if err != nil {
		return
	}

	e = Emoji{}
	if err = msgpack.Unmarshal([]byte(emojiData), &e); err != nil {
		return
	}

	return
}

// Close gracefully closes sessions and ensures all messages are sent before closing
func (m *Manager) Close() {
	m.log.Info().Msg("Closing sessions...")
	for _, s := range m.Sessions {
		s.Close()
	}

	time.Sleep(time.Second)
	for len(m.eventChannel) > 0 {
		m.log.Info().Int("event", len(m.eventChannel)).Msg("Waiting for event channel queue...")
		time.Sleep(time.Second)
	}
	close(m.eventChannel)

	time.Sleep(time.Second)
	for len(m.produceChannel) > 0 {
		m.log.Info().Int("produce", len(m.produceChannel)).Msg("Waiting for produce channel queue...")
		time.Sleep(time.Second)
	}
	close(m.produceChannel)
}
