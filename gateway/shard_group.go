package gateway

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheRockettek/Sandwich-Producer/events"
)

// ShardGroup represents a selective group of shards. Used for
// classifying a collective of shards such as during scaling.
type ShardGroup struct {
	Manager *Manager
	Scaling bool

	ShardCount int
	ShardIDs   []int

	ShardsMu sync.Mutex
	Shards   map[int]*Shard
	Wait     sync.WaitGroup
	err      error
}

// NewShardGroup makes a new shard group object for the Manager
func NewShardGroup(m *Manager, shardIDs []int, shardCount int) (sg *ShardGroup, err error) {
	m.log.Info().Int("shardcount", shardCount).Msg("Creating new ShardGroup")
	return &ShardGroup{
		Manager:    m,
		Scaling:    false,
		ShardCount: shardCount,
		ShardIDs:   shardIDs,
		ShardsMu:   sync.Mutex{},
		Shards:     make(map[int]*Shard),
		Wait:       sync.WaitGroup{},
	}, nil
}

// Spawn creates a new Shard for the ShardGroup
func (sg *ShardGroup) Spawn(shardID int) (s *Shard, err error) {
	s = &Shard{
		Manager:    sg.Manager,
		ShardGroup: sg,

		done: &sync.WaitGroup{},

		Token:      sg.Manager.Token,
		ShardID:    shardID,
		ShardCount: sg.ShardCount,

		LastHeartbeatAck:  time.Now().UTC(),
		LastHeartbeatSent: time.Now().UTC(),

		msg: events.ReceivedPayload{},
		buf: make([]byte, 0),

		seq: new(int64),
	}

	// Now we have added the Shard to the group, we can now start it up
	// and wait for it to be ready.
	sg.ShardsMu.Lock()
	sg.Shards[shardID] = s
	sg.ShardsMu.Unlock()
	s.done.Add(1)
	go s.Open()
	err = s.WaitForReady()
	return sg.Shards[shardID], err
}

// Start creates the Shards specified in the ShardIDs. Start will return
// when all Shards have started up.
func (sg *ShardGroup) Start() (err error) {
	wg := sync.WaitGroup{}
	sg.err = nil

	for _, shardID := range sg.ShardIDs {
		wg.Add(1)
		go func(shardID int) {
			defer wg.Done()
			if _, err := sg.Spawn(shardID); err != nil {
				sg.err = err
				sg.Manager.log.Error().Err(err).Msgf("Failed to start Shard %d", shardID)
			}
		}(shardID)
	}
	wg.Wait()

	if sg.err != nil {
		// If problems occur waiting for a ShardGroup's shard to start up, we
		// will kill the entire Group
		sg.Stop()
	} else {
		// Once we have created the ShardGroup, we will close the old ShardGroup if
		// there were no problems starting up the current Shard
		sg.Manager.ShardGroupsMu.Lock()
		counter := int(atomic.LoadInt64(sg.Manager.ShardGroupsCounter)) % sg.Manager.MaxShardGroups

		// Well we cant stop a ShardGroup if none have started yet...
		if counter != 0 {
			sg.Manager.ShardGroups[counter].Stop()
			delete(sg.Manager.ShardGroups, counter)
		}

		atomic.AddInt64(sg.Manager.ShardGroupsCounter, 1)
		counter = int(atomic.LoadInt64(sg.Manager.ShardGroupsCounter)) % sg.Manager.MaxShardGroups

		sg.Manager.ShardGroups[counter] = sg
		sg.Manager.ShardGroupsMu.Unlock()
	}

	return sg.err
}

// Stop stops all Shards in the ShardGroup.
func (sg *ShardGroup) Stop() {
	for _, shard := range sg.Shards {
		shard.Close(4000)
	}
}
