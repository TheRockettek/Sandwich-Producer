package gateway

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/TheRockettek/Sandwich-Producer/client"
	"github.com/TheRockettek/Sandwich-Producer/events"
	"github.com/go-redis/redis/v8"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
	"github.com/rs/zerolog"
)

// ErrNoTokenProvided is when no token was passed to the Manager
var ErrNoTokenProvided = errors.New("no token was provided")

// ErrInvalidTokenPassed is when the token passed was not valid
var ErrInvalidTokenPassed = errors.New("invalid token was passed")

// ErrNotEnoughSessions is caused when the remanining sessions provided by /gateway/bot
// is smaller than the Shards reminaining to be deployed. This is provided for safety.
var ErrNotEnoughSessions = errors.New("not enough sesssions remaining to start manager")

type void struct{}

// Manager is used to handle all shards
type Manager struct {
	Token string
	log   zerolog.Logger

	// ShardGroups contains the shards present
	ShardGroups        map[int]*ShardGroup
	ShardGroupsMu      sync.Mutex
	ShardGroupsCounter *int64
	MaxShardGroups     int

	// Limiter for Identify ratelimits and ConcurrentClients ratelimit
	ReadyLimiter *ConcurrencyLimiter

	// The HTTP client used for REST requests
	Client *client.Client

	RedisClient *redis.Client
	NatsClient  *nats.Conn
	StanClient  stan.Conn
	ctx         context.Context

	Features      Features
	Configuration Configuration

	// We will store the /gateway/bot object for future use
	Gateway *events.GatewayBot

	// Buckets will store a map that stores the different limiters
	Buckets *BucketStore
}

// Features allows for tweaking extra features normally not available
// with the default gateway
type Features struct {
	// CacheMembers will store the member object within the state. This is
	//recommended to be enabled but not necessary.
	CacheMembers bool `json:"cache_members"`

	// StoreMutuals will create a set within the state to store all guilds
	//the member can currently be seen on. This is useful for specific
	//circumstances but it is recommended to still use the oauth flow to
	//request guilds on a web dashboard, for instance. Mutuals will not be
	// stored for bots.
	StoreMutuals bool `json:"store_mutuals"`

	// IgnoreBots will not pass events that belong to bots so you do not
	//have to deal with events that are likely to be ignored anyway.
	IgnoreBots bool `json:"ignore_bots"`

	// CheckPrefix allows to ignore events that do not have a specific
	// prefix. When MESSAGE_CREATE is seen, the hashset
	// {REDIS_PREFIX}:prefix with the key being the guild id and if there
	// is an element present, if the message does not start with the
	// prefix, the message will not be forwarded. If CheckPrefixMention
	// is true, it will also pass messages if they mention the bot instead
	// of the prefix.
	CheckPrefix        bool `json:"check_prefix"`
	CheckPrefixMention bool `json:"check_prefix_mention"`
}

// Configuration stores the clients and any other configurations that is
// used during init
type Configuration struct {
	Token string `json:"token"`

	// MaxConcurrentIdentifies is an int of how many sessions can identify
	// at the same time. It is recommended to have more than 1 but not
	// too high if you do not want to use up all CPU and bottleneck which
	// can cause sessions to fail to recognise heartbeat acknowledgements
	// then fall into a reconnect loop. It is recommended that this is
	// NOT lower than your
	MaxConcurrentIdentifies int `json:"concurrent_identifies"`

	// MaxHeartbeatFailures is the ammount of heartbeats that are failed to ACK
	// before a reconnect is started
	MaxHeartbeatFailures int `json:"max_heartbeat_failures"`

	AutoSharded bool `json:"autoshard"`
	ShardCount  int  `json:"shard_count"`

	ClusterCount int `json:"cluster_count"`
	ClusterID    int `json:"cluster_id"`

	Redis struct {
		Address  string `json:"address"`
		Password string `json:"password"`
		Database int    `json:"database"`
		Prefix   string `json:"prefix"`
	} `json:"redis"`

	Nats struct {
		Address   string `json:"address"`
		Channel   string `json:"channel"`
		ClusterID string `json:"cluster"`
		ClientID  string `json:"client"`
	} `json:"nats"`

	// We will be using EventBlacklist for Sessions but we retrieve from
	// our config as a slice of strings which we will convert to after
	// loading the config.Referencing from a map is much quicker than
	// iterating a slice and checking each one.

	// Tested with GOMAXPROCS of 1 with a Ryzen 3600 and 3000Mhz memory.
	// Faster or lower latency memory will definitely affect how quick it
	// can do each iteration so depending on the memory, the MapBool can
	// be a little faster than MapVoid or when using more processes.

	// Name                    Iterations              Duration
	// BenchmarkSlice20        26642865                39.0 ns/op
	// BenchmarkMapVoid20      99909248                12.0 ns/op
	// BenchmarkMapBool20      92749807                12.7 ns/op

	// For example, using Dual Intel Xeon E5-2670 v2 with 20 threads each
	// outputs:

	// Name                      Iterations              Duration
	// BenchmarkMapVoid20-40     857180247                1.75 ns/op
	// BenchmarkMapBool20-40     939424273                1.28 ns/op

	EventBlacklist       map[string]void
	EventBlacklistValues []string `json:"event_blacklist"`

	ProduceBlacklist       map[string]void
	ProduceBlacklistValues []string `json:"produce_blacklist"`

	// Global Shard Identify Options
	Compression        bool             `json:"compression"`
	LargeThreshold     int              `json:"large_threshold"`
	DefaultPresence    *events.Activity `json:"default_activity"`
	GuildSubscriptions bool             `json:"guild_subscriptions"`
	Intents            int              `json:"intents"`
}

// NewManager creates the manager and session
func NewManager(configuration Configuration,
	features Features, logger zerolog.Logger) (m *Manager, err error) {

	if configuration.Token == "" {
		err = ErrNoTokenProvided
		return
	}
	if configuration.MaxConcurrentIdentifies <= 0 {
		configuration.MaxConcurrentIdentifies = 1
	}

	if configuration.MaxHeartbeatFailures <= 0 {
		configuration.MaxHeartbeatFailures = 5
	}

	m = &Manager{
		Token:              configuration.Token,
		ShardGroups:        make(map[int]*ShardGroup),
		ShardGroupsMu:      sync.Mutex{},
		ShardGroupsCounter: new(int64),
		MaxShardGroups:     2,
		ReadyLimiter: NewConcurrencyLimiter(
			configuration.MaxConcurrentIdentifies,
		),
		Buckets:       NewBucketStore(),
		Client:        client.NewClient(configuration.Token),
		Features:      features,
		Configuration: configuration,
		log:           logger,
		ctx:           context.Background(),
	}

	// Construct maps for both blacklists
	for _, i := range m.Configuration.EventBlacklistValues {
		m.Configuration.EventBlacklist[i] = void{}
	}
	for _, i := range m.Configuration.ProduceBlacklistValues {
		m.Configuration.ProduceBlacklist[i] = void{}
	}

	m.RedisClient = redis.NewClient(&redis.Options{
		Addr:     m.Configuration.Redis.Address,
		Password: m.Configuration.Redis.Password,
		DB:       m.Configuration.Redis.Database,
	})

	// Verify that redis has successfully connected
	err = m.RedisClient.Ping(m.ctx).Err()
	if err != nil {
		return
	}

	m.NatsClient, err = nats.Connect(m.Configuration.Nats.Address)
	if err != nil {
		return
	}

	m.StanClient, err = stan.Connect(
		m.Configuration.Nats.ClusterID,
		m.Configuration.Nats.ClientID,
		stan.NatsConn(m.NatsClient),
	)
	if err != nil {
		return
	}

	// res, err := rediScripts.ClearKeys("welcomer:*", m)
	// println(res, err)

	return
}

// Open starts up the Manager and will start up sessions
func (m *Manager) Open() (err error) {
	res := new(events.GatewayBot)
	if err = m.Client.FetchJSON("GET", "/gateway/bot", nil, &res); err != nil {
		return
	}
	m.Gateway = res

	//          _-**--__
	//      _--*         *--__         Sandwich Producer ...
	//  _-**                  **-_
	// |_*--_                _-* _|	   Cluster: ... (...)
	// | *-_ *---_     _----* _-* |    Shards: ... (...)
	//  *-_ *--__ *****  __---* _*	   Sessions Remaining: ...
	//     *--__ *-----** ___--*       Concurrent Identifies: ... / ...
	//          **-____-**

	fmt.Printf("\n         _-**--__\n     _--*         *--__         Sandwich Producer %s\n _-**                  **-_\n|_*--_                _-* _|    Cluster: %d (%d)\n| *-_ *---_     _----* _-* |    Shards: %d (%d)\n *-_ *--__ *****  __---* _*     Sessions Remaining: %d/%d\n     *--__ *-----** ___--*      Concurrent Clients: %d / %d\n         **-____-**\n\n",
		VERSION, m.Configuration.ClusterID, m.Configuration.ClusterCount, m.Configuration.ShardCount, res.Shards, res.SessionStartLimit.Remaining, res.SessionStartLimit.Total, m.Configuration.MaxConcurrentIdentifies, m.Gateway.SessionStartLimit.MaxConcurrency)

	if m.Configuration.ShardCount*2 >= res.SessionStartLimit.Remaining {
		m.log.Warn().Msgf("Current set shard count of %d is near the remaining session limit of %d",
			m.Configuration.ShardCount, res.SessionStartLimit.Remaining)
	}

	var shardCount int

	if m.Configuration.AutoSharded || m.Configuration.ShardCount < int(res.Shards)/2 {
		shardCount = res.Shards
	} else {
		shardCount = m.Configuration.ShardCount
	}

	// We will always round up the Shards to the nearest 16 if it uses more than 63 shards
	// just in order to support the majority of larger bots as we don't really know when
	// big bot sharding has occured and usually the determined devision is 16 or a multiple.
	if shardCount > 63 {
		shardCount = int(math.Ceil(float64(shardCount)/16)) * 16
	}

	m.log.Info().Msgf("Using %d shard(s)", shardCount)

	err = m.Scale(m.CreateShardIDs(shardCount), shardCount)
	return
}

// Close stops all running ShardGroups
func (m *Manager) Close() {
	m.log.Info().Msg("Closing manager")
	for _, sg := range m.ShardGroups {
		sg.Stop()
	}
}

// WaitForIdentifyRatelimit waits for a position to identify a sesssion.
// This does this whilst respecting the max_concurrency sent in the
// /gateway/bot request
func (m *Manager) WaitForIdentifyRatelimit(shardID int) {
	m.Buckets.CreateWaitForBucket(
		fmt.Sprintf("/gateway/bot/%d", shardID%m.Gateway.SessionStartLimit.MaxConcurrency),
		1,
		5*time.Second,
	)
}

// GatewayScale creates a new shard group and stops any existing ones once it has
// finished starting up. This will also fetch the gateway guilds count and
// overwrite the Gateway item on the Manager object.
func (m *Manager) GatewayScale() (err error) {
	res := new(events.GatewayBot)
	if err = m.Client.FetchJSON("GET", "/gateway/bot", nil, &res); err != nil {
		return
	}
	if res.Shards > 63 {
		res.Shards = int(math.Ceil(float64(res.Shards)/16)) * 16
	}
	m.Gateway = res

	err = m.Scale(m.CreateShardIDs(m.Gateway.Shards), m.Gateway.Shards)
	return
}

// Scale creates a new shard group and stops any existing ones once it has
// finished starting up
func (m *Manager) Scale(shardIDs []int, shardCount int) (err error) {
	sg, err := NewShardGroup(m, shardIDs, shardCount)
	if err != nil {
		return
	}

	err = sg.Start()
	return
}

// CreateShardIDs returns a slice of shard ids the bot will use
func (m *Manager) CreateShardIDs(shardCount int) (shardIDs []int) {
	deployedShards := shardCount / m.Configuration.ClusterCount
	for i := (deployedShards * m.Configuration.ClusterID); i < (deployedShards * (m.Configuration.ClusterID + 1)); i++ {
		shardIDs = append(shardIDs, i)
	}
	return
}

// // Unavailables is used to detect whether a guild has invited the bot
// // or is the initial guild object during a GUILD_CREATE event. This
// // map is stored for all sessions to use.
// Unavailables  map[int]bool
// UnavailableMu sync.RWMutex

// Manager:

// Scale(shardCount)
// this will make new shards! it will create new sessions and
// once they have all gone ready then the old shards are DC'd
// and the new shards take their place. New shards will have scale attribute true
// which will be set to false as soon as the old shards have been killed
// as to ensure no duplicate messages but likelyhood of a few missed messages.
// (better than losing them all when you manually reshard lol)
// Once you start a scale, you cannot rescale until it is done
// New Shards created during scaling will not handle events like
// MESSAGE_CREATE, but important events such as READY and GUILD_CREATE
// are necessary scaling is a boolean operator in the session
// which will define this behaviour if it should only handle events like GUILD_CREATE

// We could add a FEATURE that checks the recommended shards every 4 hours and if
// the recommended shards is greater than our current shards * MAX_GUILDS_PER_SHARD,
// then we can scale up. MAX_GUILDS_PER_SHARD simply is how many guilds do you want per shard
// before it would scale up, so by default this is ~1000 from discord so if the bot grows 50%,
// each shard will have ~1500 guilds.

// If a bot was on 5 shards with 5000 guilds and has grown to 7000,
// (7 * 1000) > (5 * 1500) will be used to see if it is good to scale up.
// This takes the recommended discord count which is arround 1 shard per 1000
// and if it is accomodates more users than our current shard count and how
// many we want on a single shard, we should scale up.
// We can also negative to figure out how many guilds are left until it scales up. For example,
// (7 * 1000) - (4 * 1500) = 1000 meaning 1000 more guilds are needed until it will surpass the
// MAX_GUILDS_PER_SHARD threshold

// SessionEvents:
// readPacket()
// 	- use a sync pool to store the object we will unmarshal to as it reduces allocs :) with ReceivedPayload
// 	- unmarshal
// 	- clear packet.Event if op isnt equal to dispatch (as we reuse object)
// 	- handlePacket()
// 	- if fn != nil then run fn

// expectPacket()
// 	- call readPacket with a function that checks Op, event and shits

// handlePacket():
// 	- switch for different Ops
// 		- dispatch
// 		- ACK
// 		- heartbeat
// 		- reconnect
// 		- invalid session

// handleDispatch()
// 	- switch for event
// 	- this is marshaler territory

// We want to create a zr gozstd which will reset and will write to an internal shard specific buffer
// https://github.com/tatsuworks/gateway/blob/master/internal/gatewayws/ws.go#L140

// FUNCTION EVERYTHING REEE
