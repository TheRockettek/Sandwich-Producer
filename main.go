package main

import (
	"context"
	"flag"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary
var zlog = zerolog.New(zerolog.ConsoleWriter{
	Out:        os.Stdout,
	TimeFormat: time.Stamp,
}).With().Timestamp().Logger()

var ctx = context.Background()

func init() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func main() {
	var err error
	token := flag.String("token", "", "token the bot will use to authenticate")
	flag.Parse()

	pass, err := ioutil.ReadFile("REDIS_PASSWORD")
	redisPassword := strings.TrimSpace(string(pass))
	zlog.Info().Msgf("using redis password: '%s'", redisPassword)

	m := NewManager(
		*token,
		"welcomer",
		managerConfiguration{
			NatsAddress: "127.0.0.1:4222",
			NatsChannel: "welcomer",
			ClientID:    "welcomer",
			ClusterID:   "cluster",
			RedisPrefix: "welcomer",
			ShardCount:  1,
			Features: features{
				CacheMembers: true,
			},
			IgnoredEvents: []string{"PRESENCE_UPDATE", "TYPING_START"},
			redisOptions: &redis.Options{
				Addr:     "127.0.0.1:6379",
				Password: redisPassword,
				DB:       0,
			},
		},
		zlog,
		UpdateStatusData{
			Game: &Game{
				Name: "welcomer.gg | +help",
			},
		},
	)
	err = m.ClearCache()
	if err != nil {
		zlog.Panic().Err(err).Msg("Could not clear cache")
	}

	err = m.Open()
	if err != nil {
		zlog.Panic().Err(err).Msg("Cold not start manager")
	}

	zlog.Info().Msg("Sessions have now started. Do ^C to close sessions")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	m.Close()
}
