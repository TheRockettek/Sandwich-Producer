package main

import (
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/TheRockettek/Sandwich-Producer/gateway"
	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog"
)

const config = `
{
    "token": "MzQyNjg1ODA3MjIxNDA3NzQ0.XuFUXg.R2_YMJm9tVx7W0RW264Nv___ovQ",
    "_token": "MzMwNDE2ODUzOTcxMTA3ODQw.XtrkdQ.QsE4ljXRHahwGfDqm7_1n2CK69I",
    "concurrent_clients":1,
    "autoshard":false,
    "shard_count":2,
    "cluster_count":1,
    "cluster_id":0,
    "max_heartbeat_failures":5,
    "redis":{
        "address":"127.0.0.1:6379",
        "password":"",
        "database":0,
        "prefix":"welcomer"
    },
    "nats":{
        "address":"127.0.0.1:4222",
        "channel":"welcomer",
        "cluster":"cluster",
        "client":"welcomer"
    },
    "event_blacklist":[

    ],
    "produce_blacklist":[

    ],

    "compression": true,
    "large_threshold": 100,
    "default_activity": {},
    "guild_subscriptions": false
}
`

func init() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
}

func main() {

	configuration := gateway.Configuration{}
	jsoniter.Unmarshal([]byte(config), &configuration)

	configuration.Nats.ClientID += "-" + strconv.Itoa(rand.Intn(9999))
	logger.Info().Msgf("Using client id %s", configuration.Nats.ClientID)

	m, err := gateway.NewManager(configuration, gateway.Features{}, logger)
	if err != nil {
		panic(err)
	}

	err = m.Open()
	if err != nil {
		panic(err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	m.Close()
	println("\nsuccess\n")
}
