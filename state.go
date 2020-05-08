// Sandwich
// Available at https://github.com/TheRockettek/Sandwich-Producer

// Copyright 2020 TheRockettek.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains code related to state tracking.  If enabled, state
// tracking will capture the initial READY packet and many other websocket
// events and maintain an in-memory state of of guilds, channels, users, and
// so forth.  This information can be accessed through the Session.State struct.

package main

import (
	"errors"
	"fmt"
	"log"
	"sort"

	"github.com/go-redis/redis"
	"github.com/vmihailenco/msgpack"
)

// ErrNilState is returned when the state is nil.
var ErrNilState = errors.New("state not instantiated, please use discordgo.New() or assign Session.State")

// ErrStateNotFound is returned when the state cache
// requested is not found
var ErrStateNotFound = errors.New("state cache not found")

func (s *Session) createMemberMap(guild *Guild) {
	values := make(map[string]interface{})
	for _, m := range guild.Members {
		_m, err := msgpack.Marshal(m)
		if err != nil {
			s.log.Error().Err(err).Msg("failed to marshal member")
			continue
		}
		values[m.User.ID] = _m
	}
	s.connections.RedisClient.HMSet(fmt.Sprintf("%s:guild:%s:members", s.connections.RedisPrefix, guild.ID), values)
}

// GuildAdd adds a guild to the current world state, or
// updates it if it already exists.
func (s *Session) GuildAdd(guild *Guild) error {
	if s == nil {
		return ErrNilState
	}

	var err error

	// s.Lock()
	// defer s.Unlock()

	marshalGuild := s.MarshalGuild(guild)

	if len(marshalGuild.Channels) > 0 {
		err = s.connections.RedisClient.HMSet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), marshalGuild.ChannelValues).Err()
		if err != nil {
			s.log.Error().Err(err).Str("id", guild.ID).Msg("failed to add guild channels on guild channel store")
		}
	}

	if len(marshalGuild.Roles) > 0 {
		err = s.connections.RedisClient.HMSet(fmt.Sprintf("%s:roles", s.connections.RedisPrefix), marshalGuild.RoleValues).Err()
		if err != nil {
			s.log.Error().Err(err).Str("id", guild.ID).Msg("failed to add guild roles on guild roles store")
		}
	}

	if len(marshalGuild.Emojis) > 0 {
		err = s.connections.RedisClient.HMSet(fmt.Sprintf("%s:emojis", s.connections.RedisPrefix), marshalGuild.EmojiValues).Err()
		if err != nil {
			s.log.Error().Err(err).Str("id", guild.ID).Msg("failed to add guild emojis on guild emojis store")
		}
	}

	_g, err := msgpack.Marshal(marshalGuild)
	if err != nil {
		log.Println(err.Error())
	}
	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild", s.connections.RedisPrefix), marshalGuild.ID, _g).Err()
	if err != nil {
		s.log.Error().Err(err).Str("id", marshalGuild.ID).Msg("failed to add guild to guild store")
	}

	return nil
}

// GuildRemove removes a guild from current world state.
func (s *Session) GuildRemove(guild *Guild) error {
	if s == nil {
		return ErrNilState
	}

	// s.Lock()
	// defer s.Unlock()

	// Remove guild channels from global cache
	for _, c := range guild.Channels {
		err := s.connections.RedisClient.HDel(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), c.ID).Err()
		if err != nil {
			s.log.Error().Err(err).Str("id", c.ID).Msg("failed to remove channel from channel store")
		}
	}

	for _, r := range guild.Roles {
		err := s.connections.RedisClient.HDel(fmt.Sprintf("%s:roles", s.connections.RedisPrefix), r.ID).Err()
		if err != nil {
			s.log.Error().Err(err).Str("id", r.ID).Msg("failed to remove role from role store")
		}
	}

	for _, e := range guild.Emojis {
		err := s.connections.RedisClient.HDel(fmt.Sprintf("%s:emojis", s.connections.RedisPrefix), e.ID).Err()
		if err != nil {
			s.log.Error().Err(err).Str("id", e.ID).Msg("failed to remove emoji from emoji store")
		}
	}

	err := s.connections.RedisClient.HDel(fmt.Sprintf("%s:guild", s.connections.RedisPrefix), guild.ID).Err()
	if err != nil {
		s.log.Error().Err(err).Str("id", guild.ID).Msg("failed to remove guild from guild store")
	}

	return nil
}

// GetGuild gets a guild by ID.
// Useful for querying if @me is in a guild:
//     _, err := discordgo.Session.State.Guild(guildID)
//     isInGuild := err == nil
func (s *Session) GetGuild(guildID string) (*Guild, error) {
	if s == nil {
		return nil, ErrNilState
	}

	// s.RLock()
	// defer s.RUnlock()

	val, err := s.connections.RedisClient.HGet(fmt.Sprintf("%s:guild", s.connections.RedisPrefix), guildID).Result()
	if err != redis.Nil {
		var g *Guild
		msgpack.Unmarshal([]byte(val), &g)

		return g, nil
	}

	return nil, ErrStateNotFound
}

// MemberAdd adds a member to the current world state, or
// updates it if it already exists.
func (s *Session) MemberAdd(member *Member) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.GetGuild(member.GuildID)
	if err != nil {
		return err
	}

	// s.Lock()
	// defer s.Unlock()

	// s.connections.RedisClient.HMSet(fmt.Sprintf("%s:guild:%s:members", s.connections.RedisPrefix, guild.ID), values)

	val, err := s.connections.RedisClient.HGet(fmt.Sprintf("%s:guild:%s:members", s.connections.RedisPrefix, guild.ID), member.User.ID).Result()

	if err == redis.Nil || err == nil {
		var m *Member
		if err == nil {
			msgpack.Unmarshal([]byte(val), &m)
			if member.JoinedAt == "" {
				member.JoinedAt = m.JoinedAt
			}
			*m = *member
		}
	}

	_m, err := msgpack.Marshal(member)
	if err != nil {
		log.Println(err.Error())
	}

	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild:%s:members", s.connections.RedisPrefix, guild.ID), member.User.ID, _m).Err()
	if err != nil {
		log.Println(err.Error())
	}

	return nil
}

// MemberRemove removes a member from current world state.
func (s *Session) MemberRemove(member *Member) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.GetGuild(member.GuildID)
	if err != nil {
		return err
	}

	// s.Lock()
	// defer s.Unlock()

	err = s.connections.RedisClient.HDel(fmt.Sprintf("%s:guild:%s:members", s.connections.RedisPrefix, guild.ID), member.User.ID).Err()
	if err != nil {
		log.Println(err.Error())
	}

	return nil
}

// GetMember gets a member by ID from a guild.
func (s *Session) GetMember(guildID, userID string) (*Member, error) {
	if s == nil {
		return nil, ErrNilState
	}

	// s.RLock()
	// defer s.RUnlock()

	val, err := s.connections.RedisClient.HGet(fmt.Sprintf("%s:guild:%s:members", s.connections.RedisPrefix, guildID), userID).Result()
	if err == redis.Nil {
		return nil, ErrStateNotFound
	}
	if err != nil {
		log.Println(err.Error())
	}

	var m *Member
	msgpack.Unmarshal([]byte(val), &m)

	return m, nil
}

// RoleAdd adds a role to the current world state, or
// updates it if it already exists.
func (s *Session) RoleAdd(guildID string, role *Role) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.GetGuild(guildID)
	if err != nil {
		return err
	}

	// s.Lock()
	// defer s.Unlock()

	for i, r := range guild.Roles {
		if r.ID == role.ID {
			guild.Roles[i] = role
			return nil
		}
	}

	guild.Roles = append(guild.Roles, role)

	_g, err := msgpack.Marshal(guild)
	if err != nil {
		log.Println(err.Error())
	}

	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild", s.connections.RedisPrefix), guild.ID, _g).Err()
	if err != nil {
		log.Println(err.Error())
	}

	return nil
}

// RoleRemove removes a role from current world state by ID.
func (s *Session) RoleRemove(guildID, roleID string) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.GetGuild(guildID)
	if err != nil {
		return err
	}

	// s.Lock()
	// defer s.Unlock()

	for i, r := range guild.Roles {
		if r.ID == roleID {
			guild.Roles = append(guild.Roles[:i], guild.Roles[i+1:]...)
			return nil
		}
	}

	_g, err := msgpack.Marshal(guild)
	if err != nil {
		log.Println(err.Error())
	}

	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild", s.connections.RedisPrefix), guild.ID, _g).Err()
	if err != nil {
		log.Println(err.Error())
	}

	return ErrStateNotFound
}

// GetRole gets a role by ID from a guild.
func (s *Session) GetRole(guildID, roleID string) (*Role, error) {
	if s == nil {
		return nil, ErrNilState
	}

	guild, err := s.GetGuild(guildID)
	if err != nil {
		return nil, err
	}

	// s.RLock()
	// defer s.RUnlock()

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
func (s *Session) ChannelAdd(channel *Channel) error {
	if s == nil {
		return ErrNilState
	}

	// s.Lock()
	// defer s.Unlock()

	val, err := s.connections.RedisClient.HGet(fmt.Sprintf("%s:guild:%s:channels", s.connections.RedisPrefix, channel.GuildID), channel.ID).Result()

	if err == redis.Nil || err == nil {
		var c *Channel
		if err == nil {
			msgpack.Unmarshal([]byte(val), &c)
			if channel.Messages == nil {
				channel.Messages = c.Messages
			}
			if channel.PermissionOverwrites == nil {
				channel.PermissionOverwrites = c.PermissionOverwrites
			}
			*c = *channel
		}
	}

	if channel.Type == ChannelTypeDM || channel.Type == ChannelTypeGroupDM {
		_c, err := msgpack.Marshal(channel)
		if err != nil {
			log.Println(err.Error())
		}
		err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild:%s:channels", s.connections.RedisPrefix, "priv"), channel.ID, _c).Err()
		if err != nil {
			log.Println(err.Error())
		}
		err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), channel.ID, _c).Err()
		if err != nil {
			log.Println(err.Error())
		}
	} else {
		_c, err := msgpack.Marshal(channel)
		if err != nil {
			log.Println(err.Error())
		}
		err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild:%s:channels", s.connections.RedisPrefix, channel.GuildID), channel.ID, _c).Err()
		if err != nil {
			log.Println(err.Error())
		}
		err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), channel.ID, _c).Err()
		if err != nil {
			log.Println(err.Error())
		}
	}

	return nil
}

// ChannelRemove removes a channel from current world state.
func (s *Session) ChannelRemove(channel *Channel) error {
	if s == nil {
		return ErrNilState
	}

	_, err := s.GetChannel(channel.ID)
	if err != nil {
		return err
	}

	if channel.Type == ChannelTypeDM || channel.Type == ChannelTypeGroupDM {
		// s.Lock()
		// defer s.Unlock()

		err := s.connections.RedisClient.HDel(fmt.Sprintf("%s:guild:%s:channels", s.connections.RedisPrefix, "priv"), channel.ID).Err()
		if err != nil {
			log.Println(err.Error())
		}
	} else {
		// s.Lock()
		// defer s.Unlock()

		err := s.connections.RedisClient.HDel(fmt.Sprintf("%s:guild:%s:channels", s.connections.RedisPrefix, channel.GuildID), channel.ID).Err()
		if err != nil {
			log.Println(err.Error())
		}
	}

	err = s.connections.RedisClient.HDel(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), channel.ID).Err()
	if err != nil {
		log.Println(err.Error())
	}

	return nil
}

// GuildChannel gets a channel by ID from a guild.
// This method is Deprecated, use Channel(channelID)
func (s *Session) GuildChannel(guildID, channelID string) (*Channel, error) {
	return s.GetChannel(channelID)
}

// PrivateChannel gets a private channel by ID.
// This method is Deprecated, use Channel(channelID)
func (s *Session) PrivateChannel(channelID string) (*Channel, error) {
	return s.GetChannel(channelID)
}

// GetChannel gets a channel by ID, it will look in all guilds and private channels.
func (s *Session) GetChannel(channelID string) (*Channel, error) {
	if s == nil {
		return nil, ErrNilState
	}

	// s.RLock()
	// defer s.RUnlock()

	val, err := s.connections.RedisClient.HGet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), channelID).Result()

	if err == nil {
		var c *Channel
		msgpack.Unmarshal([]byte(val), &c)
		return c, nil
	}

	return nil, ErrStateNotFound
}

// GetEmoji returns an emoji for a guild and emoji id.
func (s *Session) GetEmoji(guildID, emojiID string) (*Emoji, error) {
	if s == nil {
		return nil, ErrNilState
	}

	guild, err := s.GetGuild(guildID)
	if err != nil {
		return nil, err
	}

	// s.RLock()
	// defer s.RUnlock()

	for _, e := range guild.Emojis {
		if e.ID == emojiID {
			return e, nil
		}
	}

	return nil, ErrStateNotFound
}

// EmojiAdd adds an emoji to the current world state.
func (s *Session) EmojiAdd(guildID string, emoji *Emoji) error {
	if s == nil {
		return ErrNilState
	}

	guild, err := s.GetGuild(guildID)
	if err != nil {
		return err
	}

	// s.Lock()
	// defer s.Unlock()

	for i, e := range guild.Emojis {
		if e.ID == emoji.ID {
			guild.Emojis[i] = emoji
			return nil
		}
	}

	guild.Emojis = append(guild.Emojis, emoji)

	_g, err := msgpack.Marshal(guild)
	if err != nil {
		log.Println(err.Error())
	}

	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild", s.connections.RedisPrefix), guild.ID, _g).Err()
	if err != nil {
		log.Println(err.Error())
	}

	return nil
}

// EmojisAdd adds multiple emojis to the world state.
func (s *Session) EmojisAdd(guildID string, emojis []*Emoji) error {
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
func (s *Session) MessageAdd(message *Message) error {
	if s == nil {
		return ErrNilState
	}

	c, err := s.GetChannel(message.ChannelID)
	if err != nil {
		return err
	}

	// s.Lock()
	// defer s.Unlock()

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

	_c, err := msgpack.Marshal(c)
	if err != nil {
		log.Println(err.Error())
	}

	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild:%s:channels", s.connections.RedisPrefix, c.GuildID), c.ID, _c).Err()
	if err != nil {
		log.Println(err.Error())
	}

	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), c.ID, _c).Err()
	if err != nil {
		log.Println(err.Error())
	}

	return nil
}

// MessageRemove removes a message from the world state.
func (s *Session) MessageRemove(message *Message) error {
	if s == nil {
		return ErrNilState
	}

	return s.messageRemoveByID(message.ChannelID, message.ID)
}

// messageRemoveByID removes a message by channelID and messageID from the world state.
func (s *Session) messageRemoveByID(channelID, messageID string) error {
	c, err := s.GetChannel(channelID)
	if err != nil {
		return err
	}

	// s.Lock()
	// defer s.Unlock()

	for i, m := range c.Messages {
		if m.ID == messageID {
			c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)

			_c, err := msgpack.Marshal(c)
			if err != nil {
				log.Println(err.Error())
			}

			err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild:%s:channels", s.connections.RedisPrefix, c.GuildID), c.ID, _c).Err()
			if err != nil {
				log.Println(err.Error())
			}

			err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), c.ID, _c).Err()
			if err != nil {
				log.Println(err.Error())
			}
		}
	}

	return ErrStateNotFound
}

// GetMessage gets a message by channel and message ID.
func (s *Session) GetMessage(channelID, messageID string) (*Message, error) {
	if s == nil {
		return nil, ErrNilState
	}

	c, err := s.GetChannel(channelID)
	if err != nil {
		return nil, err
	}

	// s.RLock()
	// defer s.RUnlock()

	for _, m := range c.Messages {
		if m.ID == messageID {
			return m, nil
		}
	}

	return nil, ErrStateNotFound
}

// OnReady takes a Ready event and updates all internal state.
func (s *Session) onReady(r *Ready) (err error) {
	if s == nil {
		return ErrNilState
	}

	// s.Lock()
	// defer s.Unlock()

	s.sessionID = r.SessionID
	s.Ready = r
	s.IsReady = true

	// for _, g := range s.Guilds {

	// 	_g, err := msgpack.Marshal(g)
	// 	if err != nil {
	// 		log.Println(err.Error())
	// 	}

	// 	s.createMemberMap(g)

	// 	err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:guild", s.connections.RedisPrefix), g.ID, _g).Err()
	// 	if err != nil {
	// 		log.Println(err.Error())
	// 	}

	// 	for _, c := range g.Channels {
	// 		_c, err := msgpack.Marshal(c)
	// 		err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), c.ID, _c).Err()
	// 		if err != nil {
	// 			log.Println(err.Error())
	// 		}
	// 	}
	// }

	for _, c := range s.Ready.PrivateChannels {
		_c, err := msgpack.Marshal(c)
		err = s.connections.RedisClient.HSet(fmt.Sprintf("%s:channels", s.connections.RedisPrefix), c.ID, _c).Err()
		if err != nil {
			log.Println(err.Error())
		}
	}

	return nil
}

// OnInterface handles all events related to states.
func (s *Session) OnInterface(i interface{}) (err error) {
	if s == nil {
		return ErrNilState
	}

	r, ok := i.(*Ready)
	if ok {
		return s.onReady(r)
	}

	if !s.StateEnabled {
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
		guild, err := s.GetGuild(t.Member.GuildID)
		if err != nil {
			return err
		}
		guild.MemberCount++

		// Caches member if tracking is enabled.
		err = s.MemberAdd(t.Member)
	case *GuildMemberUpdate:
		err = s.MemberAdd(t.Member)
	case *GuildMemberRemove:
		// Updates the MemberCount of the guild.
		guild, err := s.GetGuild(t.Member.GuildID)
		if err != nil {
			return err
		}
		guild.MemberCount--

		err = s.MemberRemove(t.Member)
	case *GuildMembersChunk:
		for i := range t.Members {
			t.Members[i].GuildID = t.GuildID
			err = s.MemberAdd(t.Members[i])
		}
	case *GuildRoleCreate:
		err = s.RoleAdd(t.GuildID, t.Role)
	case *GuildRoleUpdate:
		err = s.RoleAdd(t.GuildID, t.Role)
	case *GuildRoleDelete:
		err = s.RoleRemove(t.GuildID, t.RoleID)
	case *GuildEmojisUpdate:
		err = s.EmojisAdd(t.GuildID, t.Emojis)
	case *ChannelCreate:
		err = s.ChannelAdd(t.Channel)
	case *ChannelUpdate:
		err = s.ChannelAdd(t.Channel)
	case *ChannelDelete:
		err = s.ChannelRemove(t.Channel)
	case *MessageCreate:
		if s.MaxMessageCount != 0 {
			err = s.MessageAdd(t.Message)
		}
	case *MessageUpdate:
		if s.MaxMessageCount != 0 {
			var old *Message
			old, err = s.GetMessage(t.ChannelID, t.ID)
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
	}

	return
}

// UserChannelPermissions returns the permission of a user in a channel.
// userID    : The ID of the user to calculate permissions for.
// channelID : The ID of the channel to calculate permission for.
func (s *Session) UserChannelPermissions(userID, channelID string) (apermissions int, err error) {
	if s == nil {
		return 0, ErrNilState
	}

	channel, err := s.GetChannel(channelID)
	if err != nil {
		return
	}

	guild, err := s.GetGuild(channel.GuildID)
	if err != nil {
		return
	}

	if userID == guild.OwnerID {
		apermissions = PermissionAll
		return
	}

	member, err := s.GetMember(guild.ID, userID)
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
func (s *Session) UserColor(userID, channelID string) int {
	if s == nil {
		return 0
	}

	channel, err := s.GetChannel(channelID)
	if err != nil {
		return 0
	}

	guild, err := s.GetGuild(channel.GuildID)
	if err != nil {
		return 0
	}

	member, err := s.GetMember(guild.ID, userID)
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
