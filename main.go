package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/go-redis/redis/v8"
	jsoniterator "github.com/json-iterator/go"
	"github.com/rs/zerolog"
)

var jsoniter = jsoniterator.ConfigCompatibleWithStandardLibrary
var zlog = zerolog.New(zerolog.ConsoleWriter{
	Out:        os.Stdout,
	TimeFormat: time.Stamp,
}).With().Timestamp().Logger()

var ctx = context.Background()

func init() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

func main() {
	var err error
	token := flag.String("token", "", "token the bot will use to authenticate")
	shardCount := flag.Int("shards", 1, "shard count to use")
	clusters := flag.Int("clusters", 1, "how many clusters are running")
	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	pass, err := ioutil.ReadFile("REDIS_PASSWORD")
	redisPassword := strings.TrimSpace(string(pass))
	zlog.Info().Msgf("using redis password: '%s'", redisPassword)

	println("Using", clusters, "cluster(s)")
	managers := make([]*Manager, 0)
	for i := 0; i < *clusters; i++ {
		println("Make cluster", i)
		m := NewManager(
			*token,
			"welcomer",
			managerConfiguration{
				NatsAddress: "127.0.0.1:4222",
				NatsChannel: "welcomer",
				ClientID:    "welcomer",
				ClusterID:   "cluster",
				RedisPrefix: "welcomer",
				ShardCount:  *shardCount,
				Features: features{
					CacheMembers: true,
					StoreMutuals: true,
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
			*clusters,
			i,
		)

		if i == 1 {
			err = m.ClearCache()
			if err != nil {
				zlog.Panic().Err(err).Msg("Could not clear cache")
			}
		}

		managers = append(managers, m)
	}

	for _, m := range managers {
		err = m.Open()
		if err != nil {
			zlog.Panic().Err(err).Msg("Cold not start manager")
		}
	}

	zlog.Info().Msg("Sessions have now started. Do ^C to close sessions")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	for _, m := range managers {
		m.Close()
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}
