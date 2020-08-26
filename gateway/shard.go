package gateway

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheRockettek/Sandwich-Producer/events"
	"github.com/TheRockettek/czlib"
	"nhooyr.io/websocket"
)

// ErrReconnectPlease is used to tell the restarter it can restart the client
var ErrReconnectPlease = errors.New("B) Can you restart the client kthx")

// Shard represents a single gateway connection
type Shard struct {
	Manager    *Manager
	ShardGroup *ShardGroup

	done   *sync.WaitGroup
	ctx    context.Context
	cancel func()

	Token      string
	ShardID    int
	ShardCount int

	LastHeartbeatAck  time.Time
	LastHeartbeatSent time.Time

	wsConn  *websocket.Conn
	wsMutex sync.Mutex

	msg events.ReceivedPayload
	buf []byte

	seq       *int64
	sessionID string
}

// Open opens the shard, this will return once the Shard has ended
func (s *Shard) Open() (err error) {
	err = s.connect()
	for s.canContinue(err) {
		err = s.connect()
	}

	s.Manager.log.Error().Int("shard", s.ShardID).Err(err).Msg("Could not continue")
	return
}

// Connect connects to the discord gateway
func (s *Shard) connect() (err error) {
	// We will now wait for any ratelimits to also be freed then
	// wait for a free spot to Identify the bot
	s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Waiting to identify")
	s.Manager.WaitForIdentifyRatelimit(s.ShardID)

	s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Waiting for concurrent session limit")
	ticket := s.Manager.ReadyLimiter.Wait()

	// TODO: FreeTicket when ready :)

	s.Manager.ReadyLimiter.FreeTicket(ticket)

	s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Ready to start")
	s.ctx, s.cancel = context.WithCancel(context.Background())
	defer s.cancel()

	// Start actually connecting
	s.Manager.log.Debug().Int("shard", s.ShardID).Msgf("Connecting to gateway")
	s.wsConn, _, err = websocket.Dial(s.ctx, s.Manager.Gateway.URL, nil)
	s.wsConn.SetReadLimit(512 << 20)

	if err != nil {
		s.Manager.log.Error().Int("shard", s.ShardID).Msg("Connecting to gateway")
		return
	}

	s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Starting gateway")

	// Expect a Hello
	err = s.readMessage()
	s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Received first message")
	if err != nil {
		s.Manager.log.Error().Int("shard", s.ShardID).Err(err).Msg("Failed to read message")
		return
	}

	hello := events.Hello{}
	err = s.decodeContent(&hello)
	if err != nil {
		s.Manager.log.Error().Int("shard", s.ShardID).Err(err).Msg("Failed to decode message")
		return
	}

	hello.HeartbeatInterval = hello.HeartbeatInterval * time.Millisecond
	ticker := time.NewTicker(hello.HeartbeatInterval)
	s.Manager.log.Debug().Int("shard", s.ShardID).Dur("heartbeat", hello.HeartbeatInterval).Msg("Received hello")

	var heartbeatFailures time.Duration
	heartbeatFailures = hello.HeartbeatInterval * (time.Duration(s.Manager.Configuration.MaxHeartbeatFailures) * time.Millisecond)

	sequence := atomic.LoadInt64(s.seq)
	if s.sessionID == "" && sequence == 0 {
		s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Sending identify packet")

		err = s.WSWriteJSON(events.SentPayload{
			Op:   2,
			Data: s.identifyPacket(),
		})
		if err != nil {
			s.Manager.log.Error().Int("shard", s.ShardID).Err(err).Msg("Failed to send identify packet")
			return
		}
	} else {
		s.Manager.log.Debug().Int("shard", s.ShardID).Str("session", s.sessionID).Int64("seq", sequence).Msg("Sending resume packet")
		err = s.WSWriteJSON(events.SentPayload{
			Op: 6,
			Data: events.Resume{
				Token:     s.Manager.Token,
				SessionID: s.sessionID,
				Seq:       sequence,
			},
		})
		if err != nil {
			s.Manager.log.Debug().Int("shard", s.ShardID).Err(err).Msg("Failed to send resume packet")
			return
		}
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Sending heartbeat")
			sequence := atomic.LoadInt64(s.seq)
			err = s.WSWriteJSON(events.SentPayload{
				Op:   int(events.GatewayOpHeartbeat),
				Data: sequence,
			})
			lastAck := s.LastHeartbeatAck
			if err != nil || time.Now().UTC().Sub(lastAck) > heartbeatFailures {
				s.Close(4000)
				return
			}
		default:
		}

		err = s.readMessage()
		if err != nil {
			s.Manager.log.Debug().Int("shard", s.ShardID).Msg("Failed to read message")
			if !s.canContinue(err) {
				return
			}
			continue
		}

		println("message!", s.ShardID, s.msg.Op, s.msg.Type, len(s.msg.Data))
	}
}

// WSWriteJSON turns an interface, marshals and sends it over WS
func (s *Shard) WSWriteJSON(i interface{}) (err error) {
	res, err := json.Marshal(i)
	if err != nil {
		return
	}
	err = s.wsConn.Write(s.ctx, websocket.MessageText, res)
	return
}

func (s *Shard) readMessage() (err error) {
	s.Manager.log.Trace().Int("shard", s.ShardID).Msg("Reading message")
	var mt websocket.MessageType

	mt, s.buf, err = s.wsConn.Read(s.ctx)
	if err != nil {
		s.Manager.log.Error().Int("shard", s.ShardID).Msg("Failed to read websocket")
		return
	}

	start := time.Now()
	defer func(s *Shard) {
		duration := time.Now().Sub(start).Milliseconds()
		if duration > 200 {
			s.Manager.log.Warn().Int("shard", s.ShardID).Int64("duration", duration).Msg("Reading a message took a while")
		}
	}(s)

	if mt == websocket.MessageBinary {
		s.buf, err = czlib.Decompress(s.buf)
		if err != nil {
			s.Manager.log.Warn().Int("shard", s.ShardID).Err(err).Msg("Failed to decompress buffer")
			return
		}
	}

	err = json.Unmarshal(s.buf, &s.msg)
	return
}

func (s *Shard) decodeContent(dat interface{}) (err error) {
	err = json.Unmarshal(s.msg.Data, &dat)
	return
}

// Close closes the websocket
func (s *Shard) Close(statusCode int) (err error) {
	s.Manager.log.Info().Int("shard", s.ShardID).Msgf("Closing shard with code %d", statusCode)

	if s.wsConn != nil {
		if err = s.wsConn.Close(4000, ""); err != nil {
			return
		}
		s.wsConn = nil
	}

	// Trigger SHARD_DISCONNECT

	return
}

// canResume returns a boolean if it is possible for the shard
// to resume
func (s *Shard) canResume() bool {
	return *s.seq != 0 && s.sessionID != ""
}

// canContinue returns a boolean if its possible to continue
// running the bot
func (s *Shard) canContinue(err error) (continuable bool) {

	continuable = err == ErrReconnectPlease || !contains(websocket.CloseStatus(err), events.CloseShardingRequired, events.CloseAuthenticationFailed, events.CloseInvalidShard, websocket.StatusNormalClosure)
	return
}

// identifyPacket returns a packet to send to discord
func (s *Shard) identifyPacket() (identify events.Identify) {
	identify = events.Identify{
		Token: s.Manager.Token,
		Properties: &events.IdentifyProperties{
			OS:      runtime.GOOS,
			Browser: "Sandwich",
			Device:  "Sandwich",
		},
		Compress:           true,
		LargeThreshold:     100,
		Shard:              [2]int{s.ShardID, s.ShardCount},
		Presence:           &events.Activity{},
		GuildSubscriptions: false,
		Intents:            0,
	}
	return
}

// WaitForReady will yield until the shard has started up
// and has finished lazy loading guilds and members. At the
// moment, we just have a WaitGroup.
func (s *Shard) WaitForReady() (err error) {
	// s.done.Wait()
	return
}
