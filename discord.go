// Sandwich
// Available at https://github.com/TheRockettek/Sandwich-Producer

// Copyright 2020 TheRockettek.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains high level helper functions and easy entry points for the
// entire discordgo package.  These functions are being developed and are very
// experimental at this point.  They will most likely change so please use the
// low level functions if that's a problem.

// Package discordgo provides Discord binding for Go
package main

import (
	"net/http"
	"strings"
	"time"
)

// VERSION of Sandwich, follows Semantic Versioning. (http://semver.org/)
const VERSION = "0.1"

// NewSession creates a new Discord session and will automate some startup
// tasks if given enough information to do so.
// To call new you require to provide a bot token. Bot will be prepended
// if it is not added.
func NewSession(token string) (s *Session, err error) {

	// Create an empty Session interface.
	s = &Session{
		IsReady:                false,
		Ratelimiter:            NewRatelimiter(),
		StateEnabled:           true,
		Compress:               true,
		ShouldReconnectOnError: true,
		ShardID:                0,
		ShardCount:             1,
		MaxRestRetries:         3,
		Client:                 &http.Client{Timeout: (20 * time.Second)},
		UserAgent:              "DiscordBot (https://github.com/TheRockettek/Sandwich-Producer, v" + VERSION + ")",
		sequence:               new(int64),
		LastHeartbeatAck:       time.Now().UTC(),
	}

	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}
	s.Token = token
	return
}
