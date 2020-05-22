// Sandwich
// Available at https://github.com/TheRockettek/Sandwich-Producer

// Copyright 2020 TheRockettek.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains low level functions for interacting with the Discord
// data websocket interface.

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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

type resumePacket struct {
	Op   int `json:"op"`
	Data struct {
		Token     string `json:"token"`
		SessionID string `json:"session_id"`
		Sequence  int64  `json:"seq"`
	} `json:"d"`
}

// Open creates a websocket connection to Discord.
// See: https://discordapp.com/developers/docs/topics/gateway#connecting
func (s *Session) Open() error {
	var err error

	// Prevent Open or other major Session functions from
	// being called while Open is still running.
	s.Lock()
	defer s.Unlock()

	// If the websock is already open, bail out here.
	if s.wsConn != nil {
		return ErrWSAlreadyOpen
	}

	// Get the gateway to use for the Websocket connection
	if s.gateway == "" {
		s.gateway, err = s.Gateway()
		if err != nil {
			return err
		}

		// Add the version and encoding to the URL
		s.gateway = s.gateway + "?v=" + APIVersion + "&encoding=json"
	}

	// Connect to the Gateway
	s.log.Info().Str("gateway", s.gateway).Msg("connecting to gateway")
	header := http.Header{}
	header.Add("accept-encoding", "zlib")
	s.wsConn, _, err = websocket.DefaultDialer.Dial(s.gateway, header)
	if err != nil {
		s.log.Warn().Err(err).Str("gateway", s.gateway).Msg("error connecting to gateway")
		s.gateway = "" // clear cached gateway
		s.wsConn = nil // Just to be safe.
		return err
	}

	s.wsConn.SetCloseHandler(func(code int, text string) error {
		return nil
	})

	defer func() {
		// because of this, all code below must set err to the error
		// when exiting with an error :)  Maybe someone has a better
		// way :)
		if err != nil {
			s.wsConn.Close()
			s.wsConn = nil
		}
	}()

	// The first response from Discord should be an Op 10 (Hello) Packet.
	// When processed by onEvent the heartbeat goroutine will be started.
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
	s.log.Debug().Msg("Op 10 Hello Packet received from Discord")
	s.LastHeartbeatAck = time.Now().UTC()
	var h helloOp
	if err = json.Unmarshal(e.RawData, &h); err != nil {
		err = fmt.Errorf("error unmarshalling helloOp, %s", err)
		return err
	}

	// Now we send either an Op 2 Identity if this is a brand new
	// connection or Op 6 Resume if we are resuming an existing connection.
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
		p := resumePacket{}
		p.Op = 6
		p.Data.Token = s.Token
		p.Data.SessionID = s.sessionID
		p.Data.Sequence = sequence

		s.log.Debug().Msg("sending resume packet to gateway")
		s.wsMutex.Lock()
		err = s.wsConn.WriteJSON(p)
		s.wsMutex.Unlock()
		if err != nil {
			err = fmt.Errorf("error sending gateway resume packet, %s, %s", s.gateway, err)
			return err
		}

	}

	// Now Discord should send us a READY or RESUMED packet.
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
	s.handleEvent(connectEventType, &Connect{})

	// Create listening chan outside of listen, as it needs to happen inside the
	// mutex lock and needs to exist before calling heartbeat and listen
	// go rountines.
	s.listening = make(chan interface{})

	// Start sending heartbeats and reading messages from Discord.
	go s.heartbeat(s.wsConn, s.listening, h.HeartbeatInterval)
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

				s.log.Warn().Err(err).Str("gateway", s.gateway).Msg("Error reading from gateway websocket")
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

type heartbeatOp struct {
	Op   int   `json:"op"`
	Data int64 `json:"d"`
}

type helloOp struct {
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

// FailedHeartbeatAcks is the Number of heartbeat intervals to wait until forcing a connection restart.
const FailedHeartbeatAcks time.Duration = 5 * time.Millisecond

// HeartbeatLatency returns the latency between heartbeat acknowledgement and heartbeat send.
func (s *Session) HeartbeatLatency() time.Duration {

	return s.LastHeartbeatAck.Sub(s.LastHeartbeatSent)

}

// heartbeat sends regular heartbeats to Discord so it knows the client
// is still connected.  If you do not send these heartbeats Discord will
// disconnect the websocket connection after a few seconds.
func (s *Session) heartbeat(wsConn *websocket.Conn, listening <-chan interface{}, heartbeatIntervalMsec time.Duration) {

	if listening == nil || wsConn == nil {
		return
	}

	var err error
	ticker := time.NewTicker(heartbeatIntervalMsec * time.Millisecond)
	defer ticker.Stop()

	for {
		s.RLock()
		last := s.LastHeartbeatAck
		s.RUnlock()
		sequence := atomic.LoadInt64(s.sequence)
		s.log.Debug().Int64("seq", *s.sequence).Msg("Sending gateway websocket heartbeat")
		s.wsMutex.Lock()
		s.LastHeartbeatSent = time.Now().UTC()
		err = wsConn.WriteJSON(heartbeatOp{1, sequence})
		s.wsMutex.Unlock()
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
		s.IsReady = true
		s.Unlock()

		select {
		case <-ticker.C:
			// continue loop and send heartbeat
		case <-listening:
			return
		}
	}
}

// UpdateStatusData ia provided to UpdateStatusComplex()
type UpdateStatusData struct {
	IdleSince *int   `json:"since"`
	Game      *Game  `json:"game"`
	AFK       bool   `json:"afk"`
	Status    string `json:"status"`
}

type updateStatusOp struct {
	Op   int              `json:"op"`
	Data UpdateStatusData `json:"d"`
}

func newUpdateStatusData(idle int, gameType GameType, game, url string) *UpdateStatusData {
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

// UpdateStatus is used to update the user's status.
// If idle>0 then set status to idle.
// If game!="" then set game.
// if otherwise, set status to active, and no game.
func (s *Session) UpdateStatus(idle int, game string) (err error) {
	return s.UpdateStatusComplex(*newUpdateStatusData(idle, GameTypeGame, game, ""))
}

// UpdateStreamingStatus is used to update the user's streaming status.
// If idle>0 then set status to idle.
// If game!="" then set game.
// If game!="" and url!="" then set the status type to streaming with the URL set.
// if otherwise, set status to active, and no game.
func (s *Session) UpdateStreamingStatus(idle int, game string, url string) (err error) {
	gameType := GameTypeGame
	if url != "" {
		gameType = GameTypeStreaming
	}
	return s.UpdateStatusComplex(*newUpdateStatusData(idle, gameType, game, url))
}

// UpdateListeningStatus is used to set the user to "Listening to..."
// If game!="" then set to what user is listening to
// Else, set user to active and no game.
func (s *Session) UpdateListeningStatus(game string) (err error) {
	return s.UpdateStatusComplex(*newUpdateStatusData(0, GameTypeListening, game, ""))
}

// UpdateStatusComplex allows for sending the raw status update data untouched by discordgo.
func (s *Session) UpdateStatusComplex(usd UpdateStatusData) (err error) {

	s.RLock()
	defer s.RUnlock()
	if s.wsConn == nil {
		return ErrWSNotFound
	}

	s.wsMutex.Lock()
	err = s.wsConn.WriteJSON(updateStatusOp{3, usd})
	s.wsMutex.Unlock()

	return
}

type requestGuildMembersData struct {
	GuildID string `json:"guild_id"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

type requestGuildMembersOp struct {
	Op   int                     `json:"op"`
	Data requestGuildMembersData `json:"d"`
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

	data := requestGuildMembersData{
		GuildID: guildID,
		Query:   query,
		Limit:   limit,
	}

	s.wsMutex.Lock()
	err = s.wsConn.WriteJSON(requestGuildMembersOp{8, data})
	s.wsMutex.Unlock()

	return
}

// onEvent is the "event handler" for all messages received on the
// Discord Gateway API websocket connection.
//
// If you use the AddHandler() function to register a handler for a
// specific event this function will pass the event along to that handler.
//
// If you use the AddHandler() function to register a handler for the
// "OnEvent" event then all events will be passed to that handler.
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

	s.log.Debug().Int("op", e.Operation).Int64("seq", e.Sequence).Str("type", e.Type).Send()

	// Ping request.
	// Must respond with a heartbeat packet within 5 seconds
	if e.Operation == 1 {
		s.log.Debug().Msg("sending heartbeat in response to Op1")
		s.wsMutex.Lock()
		err = s.wsConn.WriteJSON(heartbeatOp{1, atomic.LoadInt64(s.sequence)})
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
		s.Unlock()
		s.log.Debug().Msg("got heartbeat ACK")
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

	if !belongsToList(e.Type, s.configuration.EventBlacklist) {
		// Map event to registered event handlers and pass it along to any registered handlers.
		if eh, ok := registeredInterfaceProviders[e.Type]; ok {
			e.Struct = eh.New()

			if err = json.Unmarshal(e.RawData, e.Struct); err != nil {
				s.log.Warn().Err(err).Str("type", e.Type).Msg("error unmarshalling event")
				return nil, err
			}
			s.handleEvent(e.Type, e.Struct)
		} else {
			s.log.Warn().Int("op", e.Operation).Int64("seq", e.Sequence).Str("type", e.Type).Str("data", string(e.RawData)).Msg("unknown event")
		}

		if !belongsToList(e.Type, s.configuration.IgnoredEvents) {
			s.EventChannel <- *e
		} else {
			s.log.Debug().Str("event", e.Type).Msg("Not publishing event as it is ignored")
		}
	} else {
		s.log.Debug().Str("event", e.Type).Msg("Completely ignoring event as it is blacklisted")
	}

	return e, nil
}

type identifyProperties struct {
	OS              string `json:"$os"`
	Browser         string `json:"$browser"`
	Device          string `json:"$device"`
	Referer         string `json:"$referer"`
	ReferringDomain string `json:"$referring_domain"`
}

type identifyData struct {
	Token          string             `json:"token"`
	Properties     identifyProperties `json:"properties"`
	LargeThreshold int                `json:"large_threshold"`
	Compress       bool               `json:"compress"`
	Shard          *[2]int            `json:"shard,omitempty"`
}

type identifyOp struct {
	Op   int          `json:"op"`
	Data identifyData `json:"d"`
}

// identify sends the identify packet to the gateway
func (s *Session) identify() error {

	properties := identifyProperties{runtime.GOOS,
		"Discordgo v" + VERSION,
		"",
		"",
		"",
	}

	data := identifyData{s.Token,
		properties,
		250,
		s.Compress,
		nil,
	}

	if s.ShardCount > 1 {

		if s.ShardID >= s.ShardCount {
			return ErrWSShardBounds
		}

		data.Shard = &[2]int{s.ShardID, s.ShardCount}
	}

	op := identifyOp{2, data}

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

	s.IsReady = false

	if s.listening != nil {
		s.log.Debug().Msg("closing listening channel")
		close(s.listening)
		s.listening = nil
	}

	if s.wsConn != nil {

		s.log.Debug().Msg("sending close frame")
		// To cleanly close a connection, a client should send a close
		// frame and wait for the server to close the connection.
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
	s.handleEvent(disconnectEventType, &Disconnect{})

	return
}

// Close closes a websocket and stops all listening/heartbeat goroutines.
func (s *Session) Close() (err error) {
	s.CloseWithStatus(websocket.CloseNormalClosure)

	return
}