package main

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

const (
	// APIVersion we will use from discord
	APIVersion = "6"

	// VERSION of sandwich library
	VERSION = "0.1"

	// EndpointDiscord denotes the base URL for all api requests
	EndpointDiscord = "https://discord.com/"

	// EndpointAPI is the url subset for getting the actual API base url
	EndpointAPI = EndpointDiscord + "api/v" + APIVersion + "/"

	// EndpointGatewayBot is the URL path for retrieving the recommended shards and gateway url
	EndpointGatewayBot = EndpointAPI + "gateway/bot"
)

// ErrWSAlreadyOpen is thrown when you attempt to open
// a websocket that already is open.
var ErrWSAlreadyOpen = errors.New("web socket already opened")

// ErrWSNotFound is thrown when you attempt to use a websocket
// that doesn't exist
var ErrWSNotFound = errors.New("no websocket connection exists")

// ErrWSShardBounds is thrown when you try to use a shard ID that is
// less than the total shard count
var ErrWSShardBounds = errors.New("ShardID must be less than ShardCount")

// ErrGatewayNotFound is thrown when no gateway is specified. We expect the
// session manager to retrieve this value.
var ErrGatewayNotFound = errors.New("no gateway was passed")

// ErrInvalidToken is passed when the token used to authenticate is not valid.
var ErrInvalidToken = errors.New("Invalid token passed")

// FailedHeartbeatAcks is the Number of heartbeat intervals to wait until forcing a connection restart.
const FailedHeartbeatAcks time.Duration = 5 * time.Millisecond

// Session representation for a single shard
type Session struct {
	// Prevent other major Session functions being called
	sync.RWMutex

	// Authentication token
	Token string

	// Should the sesion reconnect on errors
	ShouldReconnectOnError bool

	// Should the session request compressed websocket data.
	Compress bool

	// Sharding
	ShardID    int
	ShardCount int

	// Whether the Data Websocket is ready. This used for voice reconnection logic
	DataReady bool

	// The http client used for REST requests
	Client *http.Client

	// The user agent used for REST APIs
	UserAgent string

	// Stores the last HeartbeatAck that was recieved (in UTC)
	LastHeartbeatAck time.Time

	// Stores the last Heartbeat sent (in UTC)
	LastHeartbeatSent time.Time

	// Presence the bot will start with
	Presence UpdateStatusData

	// The websocket connection.
	wsConn *websocket.Conn

	// When nil, the session is not listening.
	listening chan interface{}

	// sequence tracks the current gateway api websocket sequence number
	sequence *int64

	// stores sessions current Discord Gateway
	gateway string

	// stores session ID of current Gateway connection
	sessionID string

	// used to make sure gateway websocket writes do not happen concurrently
	wsMutex sync.Mutex

	// logging interface
	log *zerolog.Logger

	// map of guilds that are unavailable to differenticate joins and creations.
	// the bool dictates if the guild was already in cache to know if the guild
	// was previously down. Initial guild creates will make this false and will
	// be true if the guild goes down during runtime.
	Unavailables map[string]bool

	// channel that is shared with all shards to pipe to manager
	eventChannel chan Event
}

// NewSession creates a new session from a token, shardid and shardcount
func NewSession(token string, shardid int, shardcount int, eventChannel chan Event, log *zerolog.Logger, gateway string) (s *Session) {
	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}

	s = &Session{
		DataReady:              false,
		Compress:               true,
		ShouldReconnectOnError: true,
		ShardID:                shardid,
		ShardCount:             shardcount,
		Client:                 &http.Client{Timeout: (20 * time.Second)},
		UserAgent:              "DiscordBot (https://github.com/TheRockettek/Sandwich-Producer, v" + VERSION + ")",
		sequence:               new(int64),
		LastHeartbeatAck:       time.Now().UTC(),
		Token:                  token,
		eventChannel:           eventChannel,
		gateway:                gateway,
		log:                    log,
	}

	return
}

// Open starts up the session and will connect to gateway and start listening
func (s *Session) Open() error {
	var err error

	// Prevent this or other important functions from
	// being called again once it is running.
	s.Lock()
	defer s.Unlock()

	// If the websock is already open, we should not reopen.
	if s.wsConn != nil {
		return ErrWSAlreadyOpen
	}

	// Get the gateway to use for the Websocket connection
	if s.gateway == "" {
		s.log.Error().Err(ErrGatewayNotFound).Msg("error starting session")
		s.wsConn = nil // remove ws just incase.
		return ErrGatewayNotFound
	}

	// Connect to the Gateway
	s.log.Info().Str("gateway", s.gateway).Msg("connecting to gateway")

	header := http.Header{}
	header.Add("accept-encoding", "zlib")

	s.wsConn, _, err = websocket.DefaultDialer.Dial(s.gateway, header)
	if err != nil {
		s.log.Error().Err(err).Str("gateway", s.gateway).Msg("error connecting to gateway")
		s.wsConn = nil // remove ws just incase.
		return err
	}

	s.wsConn.SetCloseHandler(func(code int, text string) error {
		return nil
	})

	defer func() {
		// when exiting, err must be set and will then close
		if err != nil {
			s.wsConn.Close()
			s.wsConn = nil
		}
	}()

	mt, m, err := s.wsConn.ReadMessage()
	if err != nil {
		return err
	}
	e, err := s.onEvent(mt, m)
	if err != nil {
		return err
	}

	if e.Operation != 10 {
		err = fmt.Errorf("expecting Op 10, got Op %d instead", e.Operation)
		return err
	}
	s.log.Debug().Msg("Op 10 packet received from discord")

	s.LastHeartbeatAck = time.Now().UTC()

	var h Hello
	if err = json.Unmarshal(e.RawData, &h); err != nil {
		err = fmt.Errorf("error unmarshalling Hello, %s", err)
		return err
	}

	// We now have to either Resume or Identify.
	sequence := atomic.LoadInt64(s.sequence)
	if s.sessionID == "" && sequence == 0 {
		// Send Op 2 Identity Packet
		err = s.identify()
		if err != nil {
			err = fmt.Errorf("error sending identify packet to gateway, %s, %s", s.gateway, err)
			return err
		}
	} else {
		// Send Op 6 Resume Packet
		p := Resume{
			Op: 6,
			Data: struct {
				Token     string `json:"token" msgpack:"token"`
				SessionID string `json:"session_id" msgpack:"session_id"`
				Sequence  int64  `json:"seq" msgpack:"seq"`
			}{
				Token:     s.Token,
				SessionID: s.sessionID,
				Sequence:  sequence,
			},
		}
		s.log.Debug().Msg("sending resume packet to gateway")

		s.wsMutex.Lock()
		err = s.wsConn.WriteJSON(p)
		s.wsMutex.Unlock()

		if err != nil {
			err = fmt.Errorf("error sending gateway resume packet, %s, %s", s.gateway, err)
			return err
		}
	}

	// Discord should now send a READY or RESUMED packet.
	mt, m, err = s.wsConn.ReadMessage()
	if err != nil {
		return err
	}
	e, err = s.onEvent(mt, m)
	if err != nil {
		return err
	}
	s.log.Debug().Interface("packet", e).Msg("First Packet:")

	s.log.Debug().Msg("We are now connected to Discord, emitting connect event")

	s.dispatch("SHARD_READY", &Event{
		Type: "SHARD_READY",
		Data: struct {
			ShardID int `msgpack:"shard_id"`
		}{
			ShardID: s.ShardID,
		},
	})

	// Create listening chan outside of listen, as it needs to happen inside the
	// mutex lock and needs to exist before calling heartbeat and listen
	// go rountines.
	s.listening = make(chan interface{})

	// Start sending heartbeats and reading messages from Discord.
	go s.heartbeat(s.listening, h.HeartbeatInterval)
	go s.listen(s.wsConn, s.listening)
	return nil
}

// listen polls the websocket connection for events, it will stop when the
// listening channel is closed, or an error occurs.
func (s *Session) listen(wsConn *websocket.Conn, listening <-chan interface{}) {
	for {
		messageType, message, err := wsConn.ReadMessage()
		if err != nil {
			// Detect if we have been closed manually. If a Close() has already
			// happened, the websocket we are listening on will be different to
			// the current session.
			s.RLock()
			sameConnection := s.wsConn == wsConn
			s.RUnlock()

			if sameConnection {
				s.log.Error().Err(err).Str("gateway", s.gateway).Msg("Error reading from gateway websocket")
				// There has been an error reading, close the websocket so that
				// OnDisconnect event is emitted.
				err := s.Close()
				if err != nil {
					s.log.Warn().Err(err).Msg("Error closing session connection")
				}
				s.log.Debug().Msg("Calling reconnect() now")
				s.reconnect()
			}
			return
		}

		select {
		case <-listening:
			return
		default:
			s.onEvent(messageType, message)
		}
	}
}

// heartbeat sends regular heartbeats to Discord so it knows the client
// is still connected.  If you do not send these heartbeats Discord will
// disconnect the websocket connection after a few seconds.
func (s *Session) heartbeat(listening <-chan interface{}, heartbeatIntervalMsec time.Duration) {
	if listening == nil || s.wsConn == nil {
		return
	}

	var err error
	ticker := time.NewTicker(heartbeatIntervalMsec * time.Millisecond)
	defer ticker.Stop()

	for {
		sequence := atomic.LoadInt64(s.sequence)
		s.log.Debug().Int("shard", s.ShardID).Int64("seq", sequence).Msg("Sending gateway websocket heartbeat")
		s.wsMutex.Lock()
		s.LastHeartbeatSent = time.Now().UTC()
		err = s.wsConn.WriteJSON(Heartbeat{1, sequence})
		s.wsMutex.Unlock()

		s.RLock()
		last := s.LastHeartbeatAck
		s.RUnlock()

		if time.Now().UTC().Sub(last) > (heartbeatIntervalMsec * time.Millisecond) {
			s.log.Trace().Time("since", last).Msgf("Shard %d not received heartbeat ack in a while!", s.ShardID)
		}

		if err != nil || time.Now().UTC().Sub(last) > (heartbeatIntervalMsec*FailedHeartbeatAcks) {
			if err != nil {
				s.log.Error().Str("gateway", s.gateway).Err(err).Msg("Error sending heartbeat to gateway")
			} else {
				s.log.Error().Dur("duration", time.Now().UTC().Sub(last)).Msg("haven't gotten heartbeat ACK, triggering reconnection")
			}
			s.Close()
			s.reconnect()
			return
		}
		s.Lock()
		s.DataReady = true
		s.Unlock()

		select {
		case <-ticker.C:
			// continue loop and send heartbeat
		case <-listening:
			return
		}
	}
}

func (s *Session) onEvent(messageType int, message []byte) (*Event, error) {
	var err error
	var reader io.Reader
	reader = bytes.NewBuffer(message)

	// If this is a compressed message, uncompress it.
	if messageType == websocket.BinaryMessage {

		z, err2 := zlib.NewReader(reader)
		if err2 != nil {
			s.log.Error().Err(err).Msg("error uncompressing websocket message")
			return nil, err2
		}

		defer func() {
			err3 := z.Close()
			if err3 != nil {
				s.log.Warn().Err(err).Msg("error closing zlib")
			}
		}()

		reader = z
	}

	// Decode the event into an Event struct.
	var e *Event
	decoder := json.NewDecoder(reader)
	if err = decoder.Decode(&e); err != nil {
		s.log.Error().Err(err).Msg("error decoding websocket message")
		return e, err
	}

	// Ping request.
	// Must respond with a heartbeat packet within 5 seconds
	if e.Operation == 1 {
		s.log.Debug().Msg("sending heartbeat in response to Op1")
		s.wsMutex.Lock()
		err = s.wsConn.WriteJSON(Heartbeat{1, atomic.LoadInt64(s.sequence)})
		s.wsMutex.Unlock()
		if err != nil {
			s.log.Error().Msg("error sending heartbeat in response to Op1")
			return e, err
		}

		return e, nil
	}

	// Reconnect
	// Must immediately disconnect from gateway and reconnect to new gateway.
	if e.Operation == 7 {
		s.log.Debug().Msg("Closing and reconnecting in response to Op7")
		s.CloseWithStatus(4000)
		s.reconnect()
		return e, nil
	}

	// Invalid Session
	// Must respond with a Identify packet.
	if e.Operation == 9 {

		s.log.Debug().Msg("sending identify packet to gateway in response to Op9")

		err = s.identify()
		if err != nil {
			s.log.Warn().Err(err).Str("gateway", s.gateway).Msg("error sending gateway identify packet")
			return e, err
		}

		return e, nil
	}

	if e.Operation == 10 {
		// Op10 is handled by Open()
		return e, nil
	}

	if e.Operation == 11 {
		s.Lock()
		s.LastHeartbeatAck = time.Now().UTC()
		s.log.Trace().Int("shard", s.ShardID).Time("time", s.LastHeartbeatAck).Msg("received heartbeat")
		s.Unlock()

		return e, nil
	}

	// Do not try to Dispatch a non-Dispatch Message
	if e.Operation != 0 {
		// But we probably should be doing something with them.
		// TEMP
		s.log.Warn().Int("op", e.Operation).Int64("seq", e.Sequence).Str("type", e.Type).Str("data", string(e.RawData)).Str("message", string(message)).Msg("unknown")
		return e, nil
	}

	// Store the message sequence
	atomic.StoreInt64(s.sequence, e.Sequence)

	s.dispatch(e.Type, e)

	return e, nil
}

// identify sends the identify packet to the gateway
func (s *Session) identify() error {
	properties := identifyProperties{
		runtime.GOOS,
		"Sandwich v" + VERSION,
		"Sandwich v" + VERSION,
	}

	data := identifyData{
		s.Token, properties, 250, s.Compress, nil, s.Presence,
	}

	if s.ShardCount > 1 {

		if s.ShardID >= s.ShardCount {
			return ErrWSShardBounds
		}

		data.Shard = &[2]int{s.ShardID, s.ShardCount}
	}

	op := Identify{2, data}

	s.wsMutex.Lock()
	err := s.wsConn.WriteJSON(op)
	s.wsMutex.Unlock()

	return err
}

func (s *Session) reconnect() {

	var err error

	if s.ShouldReconnectOnError {

		wait := time.Duration(1)

		for {
			s.log.Info().Msg("trying to reconnect to gateway")

			err = s.Open()
			if err == nil {
				s.log.Info().Msg("successfully reconnected to gateway")
				return
			}

			// Certain race conditions can call reconnect() twice. If this happens, we
			// just break out of the reconnect loop
			if err == ErrWSAlreadyOpen {
				s.log.Info().Msg("Websocket already exists, no need to reconnect")
				return
			}

			s.log.Info().Err(err).Msg("error reconnecting to gateway")

			<-time.After(wait * time.Second)
			wait *= 2
			if wait > 600 {
				wait = 600
			}
		}
	}
}

// CloseWithStatus closes a websocket with a specified status code and stops all listening/heartbeat goroutines.
func (s *Session) CloseWithStatus(statusCode int) (err error) {
	s.Lock()

	s.DataReady = false

	if s.listening != nil {
		s.log.Debug().Msg("closing listening channel")
		close(s.listening)
		s.listening = nil
	}

	if s.wsConn != nil {
		s.log.Debug().Msg("sending close frame")

		s.wsMutex.Lock()
		err := s.wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(statusCode, ""))
		s.wsMutex.Unlock()

		if err != nil {
			s.log.Warn().Err(err).Msg("error closing websocket")
		}

		s.log.Debug().Msg("closing gateway websocket")
		err = s.wsConn.Close()
		if err != nil {
			s.log.Warn().Err(err).Msg("error closing websocket")
		}
		s.wsConn = nil
	}

	s.Unlock()

	s.log.Debug().Msg("emit disconnect event")
	s.dispatch("SHARD_DISCONNECT", &Event{
		Type: "SHARD_DISCONNECT",
		Data: ShardDisconnectOp{
			ShardID:    s.ShardID,
			StatusCode: statusCode,
		},
	})

	return
}

// Close closes a websocket and stops all listening/heartbeat goroutines.
func (s *Session) Close() (err error) {
	s.CloseWithStatus(websocket.CloseNormalClosure)
	return
}

// HeartbeatLatency retrieves the round trip time between ack and sending
func (s *Session) HeartbeatLatency() time.Duration {
	return s.LastHeartbeatAck.Sub(s.LastHeartbeatSent)
}

// CreateUpdateStatusData creates the update status data structure
func CreateUpdateStatusData(idle int, gameType GameType, game, url string) *UpdateStatusData {
	usd := &UpdateStatusData{
		Status: "online",
	}

	if idle > 0 {
		usd.IdleSince = &idle
	}

	if game != "" {
		usd.Game = &Game{
			Name: game,
			Type: gameType,
			URL:  url,
		}
	}

	return usd
}

// SendUpdateStatus allows for sending the status update data.
func (s *Session) SendUpdateStatus(usd UpdateStatusData) (err error) {

	s.RLock()
	defer s.RUnlock()
	if s.wsConn == nil {
		return ErrWSNotFound
	}

	s.wsMutex.Lock()
	err = s.wsConn.WriteJSON(UpdateStatus{3, usd})
	s.wsMutex.Unlock()

	return
}

func (s *Session) dispatch(et string, e *Event) error {
	var err error
	s.eventChannel <- *e
	return err
}

// RequestGuildMembers requests guild members from the gateway
// The gateway responds with GuildMembersChunk events
// guildID  : The ID of the guild to request members of
// query    : String that username starts with, leave empty to return all members
// limit    : Max number of items to return, or 0 to request all members matched
func (s *Session) RequestGuildMembers(guildID, query string, limit int) (err error) {
	s.RLock()
	defer s.RUnlock()
	if s.wsConn == nil {
		return ErrWSNotFound
	}

	data := RequestGuildMembersData{
		GuildID: guildID,
		Query:   query,
		Limit:   limit,
	}

	s.wsMutex.Lock()
	err = s.wsConn.WriteJSON(RequestGuildMembersOp{8, data})
	s.wsMutex.Unlock()

	return
}
