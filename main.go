package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis"
	"github.com/rs/zerolog"
)

func main() {
	configuration := Configuration{}
	ConfigPath := ""
	flag.StringVar(&ConfigPath, "config", "config.json", "Path of json config file")
	flag.Parse()

	if _, err := os.Stat(ConfigPath); err == nil {
		file, _ := ioutil.ReadFile(ConfigPath)
		json.Unmarshal(file, &configuration)
	} else {
		flag.Usage()
		return
	}

	log := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.Stamp,
	}).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Create state and connect to redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     configuration.CacheAddress,
		Password: "",
		DB:       0,
	})
	defer redisClient.Close()

	connections := Connections{}
	connections.RedisClient = redisClient
	connections.RedisPrefix = configuration.Prefix

	// Copy from configuration to connections so state can access it
	connections.IgnoredEvents = configuration.IgnoredEvents
	connections.EventBlacklist = configuration.EventBlacklist

	// Matches redis keys with wildcard and clears it.
	log.Info().Msg("Clearing redis cache")
	res, err := redisClient.Keys(fmt.Sprintf("%s:*", configuration.Prefix)).Result()
	if err != nil {
		panic(err)
	}

	for _, key := range res {
		err := redisClient.Del(key).Err()
		if err != nil {
			panic(err)
		}
	}

	// Create sessions
	sessionProvider := NewProvider()
	sessionProvider.log = log

	_, err = sessionProvider.Open(configuration, connections)
	if err != nil {
		sessionProvider.log.Fatal().Err(err).Msg("Caught exception whilst opening session provider")
	}

	sessionProvider.log.Info().Msg("Sessions have now started. Do ^C to close sessions.")

	// Wait until termination before closing
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	sessionProvider.log.Info().Msg("Closing all sessions...")
	for i := range sessionProvider.AppSessions {
		sessionProvider.AppSessions[i].Close()
	}
}
