package main

import (
	"log"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"

	"github.com/vmihailenco/msgpack"
)

// NewProvider Creates a new discord service for sharding.
func NewProvider(args ...interface{}) *SessionProvider {
	return &SessionProvider{
		eventChannel: make(chan Event, 2000),
	}
}

// Open opens the service and returns a channel which all events will be sent on.
func (d *SessionProvider) Open(configuration Configuration, connections Connections) (<-chan Event, error) {
	var shardCount int
	var shardIDs []int

	go d.Receive(configuration.NatsAddress, configuration, d.eventChannel)

	g, err := NewSession(configuration.Token)
	g.gateway, err = g.Gateway()
	if err != nil {
		return nil, err
	}
	gatewayURL := g.gateway + "?v=" + APIVersion + "&encoding=json"
	d.log.Info().Str("url", gatewayURL).Msg("Retrieved gateway url")

	// Retrieves shard count and ids
	if configuration.Autosharded || configuration.ShardCount < 1 {
		d.log.Info().Msg("Automatically retrieving shard count")
		if err != nil {
			return nil, err
		}

		s, err := g.GatewayBot()
		if err != nil {
			return nil, err
		}

		shardCount = s.Shards
		shardIDs = make([]int, shardCount)

		for i := 0; i < shardCount; i++ {
			shardIDs[i] = i
		}
	} else {
		d.log.Info().Msg("Using predefined shard count")
		shardCount = configuration.ShardCount

		if len(configuration.ShardIDs) < 1 {
			shardIDs = make([]int, shardCount)

			for i := 0; i < shardCount; i++ {
				shardIDs[i] = i
			}
		} else {
			shardIDs = configuration.ShardIDs
		}
	}

	// Generate a list of shard ids
	d.log.Info().Int("shards", shardCount).Ints("shardids", shardIDs).Msg("Using shards")
	d.AppSessions = make([]*Session, len(shardIDs))

	for i := range shardIDs {
		s, err := NewSession(configuration.Token)
		if err != nil {
			return nil, err
		}
		s.gateway = gatewayURL
		s.log = d.log

		s.ShardID = i
		s.ShardCount = shardCount
		s.MaxMessageCount = 50

		s.EventChannel = d.eventChannel
		s.configuration = configuration
		s.connections = connections
		d.AppSessions[i] = s
	}

	d.AppSession = d.AppSessions[0]
	for i := range d.AppSessions {
		err := d.AppSessions[i].Open()
		if err != nil {
			d.log.Err(err).Send()
		}
	}

	return d.eventChannel, nil
}

// Receive will handle forwarding events
func (d *SessionProvider) Receive(natsAddress string, configuration Configuration, c <-chan Event) {
	ns, err := nats.Connect(natsAddress)
	if err != nil {
		panic(err)
	}

	sc, err := stan.Connect(configuration.NatsCluster, configuration.NatsCluster, stan.NatsConn(ns))
	if err != nil {
		panic(err)
	}

	for evnt := range c {

		ma, ok := marshallers.Marshalers[evnt.Type]
		if ok {
			mevnt := ma(evnt)
			v, err := msgpack.Marshal(mevnt)
			if err == nil {
				err = sc.Publish(configuration.NatsChannel, v)
				if err != nil {
					println(err.Error())
				}
			} else {
				log.Println(err.Error())
			}

		} else {
			d.log.Warn().Str("type", evnt.Type).Msg("No marshaller available for event")
			continue
		}
	}
}
