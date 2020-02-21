package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis"
)

// (guild) <guild id>
// (guild:<guild id>:member) <member id>
// (guild:<guild id>:role) <role id>

/*

data required:

bot:
	bot identifier
	bot token

	using autosharding
	shard count

	ignore events (overall)
	ignored kafka events

	manager ip
	manager port

database:
	database ip
	database port
	database table name

cache:
	cache ip
	cache port
	cache prefix

kafka:
	kafka ip
	kafka port
	kafka prefix


*/

// StartupData defines the variables that will be used
// for configs. In future will be loading from the manager.
type StartupData struct {
	// Identification is used so consumers can handle many
	// different producers at once.
	Identity string `json:"identity"`
	Token    string `json:"token"`
	Prefix   string `json:"prefix"`

	// If autosharded is false, a shard count must be provided.
	// shard_count is ignored if autosharded has been enabled.
	IsAutosharded bool  `json:"is_autosharded"`
	ShardCount    int   `json:"shard_count"`
	ShardIDs      []int `json:"shard_ids"`

	// Cache address should be for redis.
	// Database address should be for rethinkdb.
	KafkaAddress    string `json:"kafka_address"`
	CacheAddress    string `json:"cache_address"`
	ManagerAddress  string `json:"manager_address"`
	DatabaseAddress string `json:"database_address"`

	// Events that are completely ignored (from caching or distribution)
	EventBlacklist []string `json:"event_blacklist"`

	// Events which are sent to cache but not distributed
	IgnoredEvents []string `json:"ignored_events"`
}

// NewDiscord Creates a new discord service for sharding.
func NewDiscord(config StartupData, args ...interface{}) *SessionProvider {
	return &SessionProvider{
		args:         args,
		config:       config,
		eventChannel: make(chan Event, 2000),
	}
}

// Open opens the service and returns a channel which all events will be sent on.
func (d *SessionProvider) Open(args StartupData, state *State) (<-chan Event, error) {
	var shardCount int
	var shardIDs []int

	// Retrieves shard count and ids
	if args.IsAutosharded || args.ShardCount < 1 {
		log.Println("Automatically retrieving shard count")
		gateway, err := New(args, d.args...)
		if err != nil {
			return nil, err
		}
		s, err := gateway.GatewayBot()
		if err != nil {
			return nil, err
		}
		gateway.Close()
		shardCount = s.Shards
		shardIDs = make([]int, shardCount)
	} else {
		log.Println("Using predefined shard count")
		shardCount = args.ShardCount

		if len(args.ShardIDs) < 1 {
			shardIDs = make([]int, shardCount)
		} else {
			shardIDs = args.ShardIDs
		}
	}

	// Generate a list of shard ids
	log.Printf("Shard count: %d/%d\n", len(shardIDs), shardCount)

	d.AppSessions = make([]*Session, len(shardIDs))

	for i := range shardIDs {
		session, err := New(args, d.args...)
		if err != nil {
			return nil, err
		}
		session.State = state
		session.ShardCount = shardCount
		session.ShardID = i
		session.EventChannel = d.eventChannel
		session.LogLevel = LogInformational
		d.AppSessions[i] = session
	}

	d.AppSession = d.AppSessions[0]
	go d.Receive(args, d.eventChannel)

	for i := range d.AppSessions {
		d.AppSessions[i].Open()
	}

	return d.eventChannel, nil
}

// Receive will handle forwarding events
func (d *SessionProvider) Receive(args StartupData, c <-chan Event) {
	max := 0
	total := 0
	for evnt := range c {
		total++
		if len(c) > max {
			max = len(c)
		}
		_ = evnt
		// fmt.Printf("%s [%d/%d/%d]\n", evnt.Type, len(c), max, total)
	}
}

func main() {
	data := StartupData{}
	file, _ := os.Open("data.json")
	fileBytes, _ := ioutil.ReadAll(file)
	json.Unmarshal(fileBytes, &data)

	// Create state and connect to redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     data.CacheAddress,
		Password: "",
		DB:       0,
	})

	// prefix:...
	// guild:<>
	// guild:<>:member:<>
	// guild:<>:emoji:<>
	// guild:<>:role:<>
	// user:<>

	// Matches redis keys with wildcard and clears it.
	log.Println("Clearing redis cache")
	res, err := redisClient.Keys(fmt.Sprintf("%s:*", data.Prefix)).Result()
	if err != nil {
		panic(err)
	}

	for _, key := range res {
		log.Println(key)
		err := redisClient.Del(key).Err()
		if err != nil {
			panic(err)
		}
	}

	state := NewState()
	state.TrackChannels = true
	state.TrackEmojis = true
	state.TrackMembers = false
	state.TrackRoles = true
	state.TrackVoice = false
	state.Redis = redisClient
	state.RedisPrefix = data.Prefix

	// Create sessions
	session := NewDiscord(data, data.Token)
	ch, _ := session.Open(data, state)

	log.Println("Sessions have now started. Do ^C to close sessions.")

	// Wait until termination before closing
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	log.Println("Closing all sessions...")
	for i := range session.AppSessions {
		session.AppSessions[i].Close()
	}

	start := time.Now()
	for len(ch) > 0 && time.Now().Sub(start) < (time.Second*10) {
		time.Sleep(time.Second)
		log.Printf("Waiting for producer channel: %s / 10s\n", time.Now().Sub(start))
	}
}
