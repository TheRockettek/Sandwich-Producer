// Discordgo - Discord bindings for Go
// Available at https://github.com/bwmarrin/discordgo

// Copyright 2015-2016 Bruce Marriner <bruce@sqls.net>.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains high level helper functions and easy entry points for the
// entire discordgo package.  These functions are being developed and are very
// experimental at this point.  They will most likely change so please use the
// low level functions if that's a problem.

// Package discordgo provides Discord binding for Go
package main

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// VERSION of DiscordGo, follows Semantic Versioning. (http://semver.org/)
const VERSION = "0.20.2"

// ErrMFA will be risen by New when the user has 2FA.
var ErrMFA = errors.New("account has 2FA enabled")

// New creates a new Discord session and will automate some startup
// tasks if given enough information to do so.  Currently you can pass zero
// arguments and it will return an empty Discord session.
// There are 3 ways to call New:
//     With a single auth token - All requests will use the token blindly,
//         no verification of the token will be done and requests may fail.
//         IF THE TOKEN IS FOR A BOT, IT MUST BE PREFIXED WITH `BOT `
//         eg: `"Bot <token>"`
func New(data StartupData, args ...interface{}) (s *Session, err error) {

	// Create an empty Session interface.
	s = &Session{
		State:                  NewState(),
		Ratelimiter:            NewRatelimiter(),
		StateEnabled:           true,
		Compress:               true,
		ShouldReconnectOnError: true,
		ShardID:                0,
		ShardCount:             1,
		MaxRestRetries:         3,
		Client:                 &http.Client{Timeout: (20 * time.Second)},
		UserAgent:              "DiscordBot (https://github.com/bwmarrin/discordgo, v" + VERSION + ")",
		sequence:               new(int64),
		LastHeartbeatAck:       time.Now().UTC(),
		ready:                  false,
		SandwichConfig:         data,
	}

	// If no arguments are passed return the empty Session.
	if args == nil {
		return
	}

	var token string

	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			// Assume first value in arguments is a token
			if token == "" {
				token = v
			}
		}
	}

	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}

	s.Token = token
	s.log(LogDebug, "Logging in with token %s", token)
	return
}
