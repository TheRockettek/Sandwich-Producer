// Discordgo - Discord bindings for Go
// Available at https://github.com/bwmarrin/discordgo

// Copyright 2015-2016 Bruce Marriner <bruce@sqls.net>.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains code related to state tracking.  If enabled, state
// tracking will capture the initial READY packet and many other websocket
// events and maintain an in-memory state of of guilds, channels, users, and
// so forth.  This information can be accessed through the Session.State struct.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/go-redis/redis"
)

// ErrNilState is returned when the state is nil.
var ErrNilState = errors.New("state not instantiated, please use discordgo.New() or assign Session.State")

// ErrStateNotFound is returned when the state cache
// requested is not found
var ErrStateNotFound = errors.New("state cache not found")

// A State contains the current known state.
// As discord sends this in a READY blob, it seems reasonable to simply
// use that struct as the data store.
type State struct {
	sync.RWMutex
	Ready

	// Store the Redis Instance
	Redis       *redis.Client
	RedisPrefix string

	// MaxMessageCount represents how many messages per channel the state will store.
	MaxMessageCount int
	TrackChannels   bool
	TrackEmojis     bool
	TrackMembers    bool
	TrackRoles      bool
	TrackVoice      bool
	TrackPresences  bool
}

// NewState creates an empty state.
func NewState() *State {
	return &State{
		TrackChannels:  true,
		TrackEmojis:    true,
		TrackMembers:   true,
		TrackRoles:     true,
		TrackVoice:     true,
		TrackPresences: true,
	}
}

func (s *State) createMemberMap(guild *Guild) {
	values := make(map[string]interface{})
	for _, m := range guild.Members {
		_m, err := json.Marshal(m)
		if err != nil {
			fmt.Println(err)
		}
		values[m.User.ID] = _m
	}
	s.Redis.HMSet(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guild.ID), values)
}

// GuildAdd adds a guild to the current world state, or
// updates it if it already exists.
func (s *State) GuildAdd(guild *Guild) error {
	if s == nil {
		return ErrNilState
	}

	s.Lock()
	defer s.Unlock()

	// Update the channels to point to the right guild, adding them to the channelMap as we go
	values := make(map[string]interface{})
	for _, c := range guild.Channels {
		_c, err := json.Marshal(c)
		if err != nil {
			fmt.Println(err)
		}
		values[c.ID] = _c
	}
	s.Redis.HMSet(fmt.Sprintf("%s:guild:%s:channels", s.RedisPrefix, guild.ID), values)

	_g, err := json.Marshal(guild)
	if err != nil {
		fmt.Println(err)
	}

	err = s.Redis.HSet(fmt.Sprintf("%s:guilds", ""), guild.ID, _g).Err()
	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// GuildRemove removes a guild from current world state.
func (s *State) GuildRemove(guild *Guild) error {
	if s == nil {
		return ErrNilState
	}

	_, err := s.Guild(guild.ID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	err = s.Redis.HDel(fmt.Sprintf("%s:guilds", ""), guild.ID).Err()
	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// Guild gets a guild by ID.
// Useful for querying if @me is in a guild:
//     _, err := discordgo.Session.State.Guild(guildID)
//     isInGuild := err == nil
func (s *State) Guild(guildID string) (*Guild, error) {
	if s == nil {
		return nil, ErrNilState
	}

	s.RLock()
	defer s.RUnlock()

	val, err := s.Redis.HGet(fmt.Sprintf("%s:guild", ""), guildID).Result()
	if err != redis.Nil {
		var g *Guild
		json.Unmarshal([]byte(val), &g)

		return g, nil
	}

	return nil, ErrStateNotFound
}

// MemberAdd adds a member to the current world state, or
// updates it if it already exists.
func (s *State) MemberAdd(member *Member) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.Guild(member.GuildID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	// s.Redis.HMSet(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guild.ID), values)

	val, err := s.Redis.HGet(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guild.ID), member.User.ID).Result()

	if err == redis.Nil || err == nil {
		var m *Member
		if err == nil {
			json.Unmarshal([]byte(val), &m)
		}
		if member.JoinedAt == "" {
			member.JoinedAt = m.JoinedAt
		}
		*m = *member

		_m, err := json.Marshal(m)
		if err != nil {
			fmt.Println(err)
		}
		err = s.Redis.HSet(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guild.ID), member.User.ID, _m).Err()
		if err != nil {
			fmt.Println(err)
		}
	} else {
		fmt.Println(err)
	}

	return nil
}

// MemberRemove removes a member from current world state.
func (s *State) MemberRemove(member *Member) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.Guild(member.GuildID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	exists, err := s.Redis.Exists(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guild.ID)).Result()
	if err != nil {
		fmt.Println(err)
	}
	if exists == 0 {
		return ErrStateNotFound
	}

	val, err := s.Redis.HExists(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guild.ID), member.User.ID).Result()
	if err != nil {
		fmt.Println(err)
	}
	if !val {
		return ErrStateNotFound
	}

	err = s.Redis.HDel(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guild.ID), member.User.ID).Err()
	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// Member gets a member by ID from a guild.
func (s *State) Member(guildID, userID string) (*Member, error) {
	if s == nil {
		return nil, ErrNilState
	}

	s.RLock()
	defer s.RUnlock()

	val, err := s.Redis.HGet(fmt.Sprintf("%s:guild:%s:members", s.RedisPrefix, guildID), userID).Result()
	if err == redis.Nil {
		return nil, ErrStateNotFound
	}
	if err != nil {
		fmt.Println(err)
	}

	var m *Member
	json.Unmarshal([]byte(val), &m)

	return m, nil
}

// RoleAdd adds a role to the current world state, or
// updates it if it already exists.
func (s *State) RoleAdd(guildID string, role *Role) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.Guild(guildID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	for i, r := range guild.Roles {
		if r.ID == role.ID {
			guild.Roles[i] = role
			return nil
		}
	}

	guild.Roles = append(guild.Roles, role)
	return nil
}

// RoleRemove removes a role from current world state by ID.
func (s *State) RoleRemove(guildID, roleID string) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.Guild(guildID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	for i, r := range guild.Roles {
		if r.ID == roleID {
			guild.Roles = append(guild.Roles[:i], guild.Roles[i+1:]...)
			return nil
		}
	}

	return ErrStateNotFound
}

// Role gets a role by ID from a guild.
func (s *State) Role(guildID, roleID string) (*Role, error) {
	if s == nil {
		return nil, ErrNilState
	}

	guild, err := s.Guild(guildID)
	if err != nil {
		return nil, err
	}

	s.RLock()
	defer s.RUnlock()

	for _, r := range guild.Roles {
		if r.ID == roleID {
			return r, nil
		}
	}

	return nil, ErrStateNotFound
}

// ChannelAdd adds a channel to the current world state, or
// updates it if it already exists.
// Channels may exist either as PrivateChannels or inside
// a guild.
func (s *State) ChannelAdd(channel *Channel) error {
	if s == nil {
		return ErrNilState
	}

	s.Lock()
	defer s.Unlock()

	val, err := s.Redis.HGet(fmt.Sprintf("%s:guild:%s:channels", s.RedisPrefix, channel.GuildID), channel.ID).Result()

	if err == redis.Nil || err == nil {
		var c *Channel
		if err == nil {
			json.Unmarshal([]byte(val), &c)
			if channel.Messages == nil {
				channel.Messages = c.Messages
			}
			if channel.PermissionOverwrites == nil {
				channel.PermissionOverwrites = c.PermissionOverwrites
			}
			*c = *channel
		}
	}

	var _c string
	if channel.Type == ChannelTypeDM || channel.Type == ChannelTypeGroupDM {
		_c, err := json.Marshal(channel)
		if err != nil {
			fmt.Println(err)
		}
		err = s.Redis.HSet(fmt.Sprintf("%s:guild:%s:channels", s.RedisPrefix, "priv"), channel.ID, _c).Err()
		if err != nil {
			fmt.Println(err)
		}
	} else {
		_c, err := json.Marshal(channel)
		if err != nil {
			fmt.Println(err)
		}
		err = s.Redis.HSet(fmt.Sprintf("%s:guild:%s:channels", s.RedisPrefix, channel.GuildID), channel.ID, _c).Err()
		if err != nil {
			fmt.Println(err)
		}
	}

	err = s.Redis.HSet(fmt.Sprintf("%s:channels", ""), channel.ID, _c).Err()
	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// ChannelRemove removes a channel from current world state.
func (s *State) ChannelRemove(channel *Channel) error {
	if s == nil {
		return ErrNilState
	}

	_, err := s.Channel(channel.ID)
	if err != nil {
		return err
	}

	if channel.Type == ChannelTypeDM || channel.Type == ChannelTypeGroupDM {
		s.Lock()
		defer s.Unlock()

		err := s.Redis.HDel(fmt.Sprintf("%s:guild:%s:channels", s.RedisPrefix, "priv"), channel.ID).Err()
		if err != nil {
			fmt.Println(err)
		}
	} else {
		s.Lock()
		defer s.Unlock()

		err := s.Redis.HDel(fmt.Sprintf("%s:guild:%s:channels", s.RedisPrefix, channel.GuildID), channel.ID).Err()
		if err != nil {
			fmt.Println(err)
		}
	}

	err = s.Redis.HDel(fmt.Sprintf("%s:channels", ""), channel.ID).Err()
	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// GuildChannel gets a channel by ID from a guild.
// This method is Deprecated, use Channel(channelID)
func (s *State) GuildChannel(guildID, channelID string) (*Channel, error) {
	return s.Channel(channelID)
}

// PrivateChannel gets a private channel by ID.
// This method is Deprecated, use Channel(channelID)
func (s *State) PrivateChannel(channelID string) (*Channel, error) {
	return s.Channel(channelID)
}

// Channel gets a channel by ID, it will look in all guilds and private channels.
func (s *State) Channel(channelID string) (*Channel, error) {
	if s == nil {
		return nil, ErrNilState
	}

	s.RLock()
	defer s.RUnlock()

	val, err := s.Redis.HGet(fmt.Sprintf("%s:channels", ""), channelID).Result()

	if err == nil {
		var c *Channel
		json.Unmarshal([]byte(val), &c)
		return c, nil
	}

	return nil, ErrStateNotFound
}

// Emoji returns an emoji for a guild and emoji id.
func (s *State) Emoji(guildID, emojiID string) (*Emoji, error) {
	if s == nil {
		return nil, ErrNilState
	}

	guild, err := s.Guild(guildID)
	if err != nil {
		return nil, err
	}

	s.RLock()
	defer s.RUnlock()

	for _, e := range guild.Emojis {
		if e.ID == emojiID {
			return e, nil
		}
	}

	return nil, ErrStateNotFound
}

// EmojiAdd adds an emoji to the current world state.
func (s *State) EmojiAdd(guildID string, emoji *Emoji) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.Guild(guildID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	for i, e := range guild.Emojis {
		if e.ID == emoji.ID {
			guild.Emojis[i] = emoji
			return nil
		}
	}

	guild.Emojis = append(guild.Emojis, emoji)
	return nil
}

// EmojisAdd adds multiple emojis to the world state.
func (s *State) EmojisAdd(guildID string, emojis []*Emoji) error {
	for _, e := range emojis {
		if err := s.EmojiAdd(guildID, e); err != nil {
			return err
		}
	}
	return nil
}

// MessageAdd adds a message to the current world state, or updates it if it exists.
// If the channel cannot be found, the message is discarded.
// Messages are kept in state up to s.MaxMessageCount per channel.
func (s *State) MessageAdd(message *Message) error {
	if s == nil {
		return ErrNilState
	}

	c, err := s.Channel(message.ChannelID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	// If the message exists, merge in the new message contents.
	for _, m := range c.Messages {
		if m.ID == message.ID {
			if message.Content != "" {
				m.Content = message.Content
			}
			if message.EditedTimestamp != "" {
				m.EditedTimestamp = message.EditedTimestamp
			}
			if message.Mentions != nil {
				m.Mentions = message.Mentions
			}
			if message.Embeds != nil {
				m.Embeds = message.Embeds
			}
			if message.Attachments != nil {
				m.Attachments = message.Attachments
			}
			if message.Timestamp != "" {
				m.Timestamp = message.Timestamp
			}
			if message.Author != nil {
				m.Author = message.Author
			}

			return nil
		}
	}

	c.Messages = append(c.Messages, message)

	if len(c.Messages) > s.MaxMessageCount {
		c.Messages = c.Messages[len(c.Messages)-s.MaxMessageCount:]
	}
	return nil
}

// MessageRemove removes a message from the world state.
func (s *State) MessageRemove(message *Message) error {
	if s == nil {
		return ErrNilState
	}

	return s.messageRemoveByID(message.ChannelID, message.ID)
}

// messageRemoveByID removes a message by channelID and messageID from the world state.
func (s *State) messageRemoveByID(channelID, messageID string) error {
	c, err := s.Channel(channelID)
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	for i, m := range c.Messages {
		if m.ID == messageID {
			c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)
			return nil
		}
	}

	return ErrStateNotFound
}

// Message gets a message by channel and message ID.
func (s *State) Message(channelID, messageID string) (*Message, error) {
	if s == nil {
		return nil, ErrNilState
	}

	c, err := s.Channel(channelID)
	if err != nil {
		return nil, err
	}

	s.RLock()
	defer s.RUnlock()

	for _, m := range c.Messages {
		if m.ID == messageID {
			return m, nil
		}
	}

	return nil, ErrStateNotFound
}

// OnReady takes a Ready event and updates all internal state.
func (s *State) onReady(se *Session, r *Ready) (err error) {
	if s == nil {
		return ErrNilState
	}

	s.Lock()
	defer s.Unlock()

	// We must track at least the current user for Voice, even
	// if state is disabled, store the bare essentials.
	if !se.StateEnabled {
		ready := Ready{
			Version:   r.Version,
			SessionID: r.SessionID,
			User:      r.User,
		}

		s.Ready = ready

		return nil
	}

	s.Ready = *r

	for _, g := range s.Guilds {

		_g, err := json.Marshal(g)
		if err != nil {
			fmt.Println(err)
		}

		err = s.Redis.HSet(fmt.Sprintf("%s:guild", ""), g.ID, _g).Err()
		if err != nil {
			fmt.Println(err)
		}

		s.createMemberMap(g)

		for _, c := range g.Channels {
			_c, err := json.Marshal(c)
			err = s.Redis.HSet(fmt.Sprintf("%s:channels", ""), c.ID, _c).Err()
			if err != nil {
				fmt.Println(err)
			}
		}
	}

	for _, c := range s.PrivateChannels {
		_c, err := json.Marshal(c)
		err = s.Redis.HSet(fmt.Sprintf("%s:channels", ""), c.ID, _c).Err()
		if err != nil {
			fmt.Println(err)
		}
	}

	return nil
}

// OnInterface handles all events related to states.
func (s *State) OnInterface(se *Session, i interface{}) (err error) {
	if s == nil {
		return ErrNilState
	}

	r, ok := i.(*Ready)
	if ok {
		return s.onReady(se, r)
	}

	if !se.StateEnabled {
		return nil
	}

	switch t := i.(type) {
	case *GuildCreate:
		err = s.GuildAdd(t.Guild)
	case *GuildUpdate:
		err = s.GuildAdd(t.Guild)
	case *GuildDelete:
		err = s.GuildRemove(t.Guild)
	case *GuildMemberAdd:
		// Updates the MemberCount of the guild.
		guild, err := s.Guild(t.Member.GuildID)
		if err != nil {
			return err
		}
		guild.MemberCount++

		// Caches member if tracking is enabled.
		if s.TrackMembers {
			err = s.MemberAdd(t.Member)
		}
	case *GuildMemberUpdate:
		if s.TrackMembers {
			err = s.MemberAdd(t.Member)
		}
	case *GuildMemberRemove:
		// Updates the MemberCount of the guild.
		guild, err := s.Guild(t.Member.GuildID)
		if err != nil {
			return err
		}
		guild.MemberCount--

		// Removes member from the cache if tracking is enabled.
		if s.TrackMembers {
			err = s.MemberRemove(t.Member)
		}
	case *GuildMembersChunk:
		if s.TrackMembers {
			for i := range t.Members {
				t.Members[i].GuildID = t.GuildID
				err = s.MemberAdd(t.Members[i])
			}
		}
	case *GuildRoleCreate:
		if s.TrackRoles {
			err = s.RoleAdd(t.GuildID, t.Role)
		}
	case *GuildRoleUpdate:
		if s.TrackRoles {
			err = s.RoleAdd(t.GuildID, t.Role)
		}
	case *GuildRoleDelete:
		if s.TrackRoles {
			err = s.RoleRemove(t.GuildID, t.RoleID)
		}
	case *GuildEmojisUpdate:
		if s.TrackEmojis {
			err = s.EmojisAdd(t.GuildID, t.Emojis)
		}
	case *ChannelCreate:
		if s.TrackChannels {
			err = s.ChannelAdd(t.Channel)
		}
	case *ChannelUpdate:
		if s.TrackChannels {
			err = s.ChannelAdd(t.Channel)
		}
	case *ChannelDelete:
		if s.TrackChannels {
			err = s.ChannelRemove(t.Channel)
		}
	case *MessageCreate:
		if s.MaxMessageCount != 0 {
			err = s.MessageAdd(t.Message)
		}
	case *MessageUpdate:
		if s.MaxMessageCount != 0 {
			var old *Message
			old, err = s.Message(t.ChannelID, t.ID)
			if err == nil {
				oldCopy := *old
				t.BeforeUpdate = &oldCopy
			}

			err = s.MessageAdd(t.Message)
		}
	case *MessageDelete:
		if s.MaxMessageCount != 0 {
			err = s.MessageRemove(t.Message)
		}
	case *MessageDeleteBulk:
		if s.MaxMessageCount != 0 {
			for _, mID := range t.Messages {
				s.messageRemoveByID(t.ChannelID, mID)
			}
		}
	case *PresenceUpdate:
		if s.TrackMembers {
			if t.Status == StatusOffline {
				return
			}

			var m *Member
			m, err = s.Member(t.GuildID, t.User.ID)

			if err != nil {
				// Member not found; this is a user coming online
				m = &Member{
					GuildID: t.GuildID,
					Nick:    t.Nick,
					User:    t.User,
					Roles:   t.Roles,
				}

			} else {

				if t.Nick != "" {
					m.Nick = t.Nick
				}

				if t.User.Username != "" {
					m.User.Username = t.User.Username
				}

				// PresenceUpdates always contain a list of roles, so there's no need to check for an empty list here
				m.Roles = t.Roles

			}

			err = s.MemberAdd(m)
		}

	}

	return
}

// UserChannelPermissions returns the permission of a user in a channel.
// userID    : The ID of the user to calculate permissions for.
// channelID : The ID of the channel to calculate permission for.
func (s *State) UserChannelPermissions(userID, channelID string) (apermissions int, err error) {
	if s == nil {
		return 0, ErrNilState
	}

	channel, err := s.Channel(channelID)
	if err != nil {
		return
	}

	guild, err := s.Guild(channel.GuildID)
	if err != nil {
		return
	}

	if userID == guild.OwnerID {
		apermissions = PermissionAll
		return
	}

	member, err := s.Member(guild.ID, userID)
	if err != nil {
		return
	}

	return memberPermissions(guild, channel, member), nil
}

// UserColor returns the color of a user in a channel.
// While colors are defined at a Guild level, determining for a channel is more useful in message handlers.
// 0 is returned in cases of error, which is the color of @everyone.
// userID    : The ID of the user to calculate the color for.
// channelID   : The ID of the channel to calculate the color for.
func (s *State) UserColor(userID, channelID string) int {
	if s == nil {
		return 0
	}

	channel, err := s.Channel(channelID)
	if err != nil {
		return 0
	}

	guild, err := s.Guild(channel.GuildID)
	if err != nil {
		return 0
	}

	member, err := s.Member(guild.ID, userID)
	if err != nil {
		return 0
	}

	roles := Roles(guild.Roles)
	sort.Sort(roles)

	for _, role := range roles {
		for _, roleID := range member.Roles {
			if role.ID == roleID {
				if role.Color != 0 {
					return role.Color
				}
			}
		}
	}

	return 0
}
