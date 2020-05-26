package main

import (
	"encoding/json"
	"fmt"

	"github.com/vmihailenco/msgpack"
)

var marshalers = make(map[string]func(*Manager, Event) (bool, StreamEvent))

// Marshalers are used to both handle state caching and also produce a structure that is sent to consumers.
// The boolean in a marshaler dictates if that event should be directed to consumers for manual overrides.
// This only manually overrides blocking so if it returns true and it is in the blacklist, it will still be
// sent to consumers.

// addMarshaler adds a marshaler for a specific event.
func addMarshaler(event string, marshaler func(*Manager, Event) (bool, StreamEvent)) {
	if _, ok := marshalers[event]; ok {
		return
	}
	marshalers[event] = marshaler
}

func shardReadyMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	ok, se.Data = true, e.Data
	zlog.Info().Msgf("Shard %d is ready", e.Data.(struct {
		ShardID int `msgpack:"shard_id"`
	}).ShardID)
	return
}

func shardDisconnectMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	ok, se.Data = true, e.Data
	zlog.Info().Msgf("Shard %d has disconnected", e.Data.(struct {
		ShardID int `msgpack:"shard_id"`
	}).ShardID)
	return
}

func readyMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error

	ready := Ready{}
	err = json.Unmarshal(e.RawData, &ready)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal ready")
		return
	}

	for _, g := range ready.Guilds {
		m.Unavailables[g.ID] = false
	}

	ok = true

	return
}

func guildCreateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error

	guild := MarshalGuild{}
	err = guild.Create(e.RawData)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild")
		return
	}

	// As we remove the guild from m.Unavailables, if the bot resumed we do
	// not know if the bot was removed from the guild but as we store the guild
	// in cache, if the guild is in the cache we know it was not removed from
	// the guild so we can handle as it going available again. The only caveate
	// would be being removed during resume however those events should be refired.
	ic, err := m.Configuration.redisClient.HExists(
		ctx,
		fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
		guild.ID,
	).Result()

	err = guild.Save(m)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to save guild")
	}

	if err != nil {
		zlog.Error().Err(err).Msg("failed to check for guild in cache")
	}
	// Check if guild is in the pending availability map or is currently in cache.
	if un, uo := m.Unavailables[guild.ID]; uo || ic {
		// If the guild was unavailable, this means it is now available so fire the
		// available event. If it is in the cache it also means that it is likely to
		// be available again incase we did not get the unavailable payload. If neither,
		// this just means it was initial GUILD_CREATE event when the bot is connecting
		// and we can just ignore it.
		if un || ic {
			ok = true
			se = StreamEvent{
				Type: "GUILD_AVAILABLE",
				Data: &guild,
			}
		} else {
			ok = false
		}
	} else {
		// We will only fire events if they have invited bot
		ok = true
		se = StreamEvent{
			Type: "GUILD_JOIN",
			Data: &guild,
		}
	}

	return ok, se
}

func guildDeleteMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error
	var rawData string

	partialGuild := UnavailableGuild{}
	err = json.Unmarshal(e.RawData, &partialGuild)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal partial guild")
		return
	}

	guild := MarshalGuild{}
	guild.From([]byte(rawData))

	delete(m.Unavailables, partialGuild.ID)

	if partialGuild.Unavailable {
		// guild has gone down
		m.Unavailables[partialGuild.ID] = true
		ok = true
		se = StreamEvent{
			Type: "GUILD_UNAVAILABLE",
			Data: guild,
		}
	} else {
		// user has left guild
		if err = m.Configuration.redisClient.HDel(
			ctx,
			fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
			partialGuild.ID,
		).Err(); err != nil {
			zlog.Error().Err(err).Msg("failed to remove guild")
		}

		if len(guild.Roles) > 0 {
			if err = m.Configuration.redisClient.HDel(
				ctx,
				fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guild.ID),
				guild.Roles...,
			).Err(); err != nil {
				zlog.Error().Err(err).Msg("failed to remove roles")
			}
		}

		if len(guild.Channels) > 0 {
			if err = m.Configuration.redisClient.HDel(
				ctx,
				fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
				guild.Channels...,
			).Err(); err != nil {
				zlog.Error().Err(err).Msg("failed to remove channels")
			}
		}

		if len(guild.Emojis) > 0 {
			if err = m.Configuration.redisClient.HDel(
				ctx,
				fmt.Sprintf("%s:emojis", m.Configuration.RedisPrefix),
				guild.Emojis...,
			).Err(); err != nil {
				zlog.Error().Err(err).Msg("failed to remove emojis")
			}
		}

		if err = m.Configuration.redisClient.HDel(
			ctx,
			fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
			guild.ID,
		).Err(); err != nil {
			zlog.Error().Err(err).Msg("failed to remove guild")
		}

		ok = true
		se = StreamEvent{
			Type: "GUILD_REMOVE",
			Data: &guild,
		}
	}

	return
}

func guildUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error

	updatedGuild := MarshalGuild{}
	err = updatedGuild.Create(e.RawData)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild")
		return
	}

	guild, err := m.getGuild(updatedGuild.ID)
	if err != nil {
		zlog.Error().Err(err).Msgf("GUILD_UPDATE referenced unknown guild %d", updatedGuild.ID)
	}

	if err = updatedGuild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to update guild")
	}

	ok = true
	se = StreamEvent{
		Type: "GUILD_UPDATE",
		Data: struct {
			Before *MarshalGuild `msgpack:"before"`
			After  *MarshalGuild `msgpack:"after"`
		}{
			Before: &guild,
			After:  &updatedGuild,
		},
	}

	return
}

func guildRoleCreateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error

	// The GuildRoleCreate struct contains the role and guild id
	guildRole := GuildRoleCreate{}
	if err = json.Unmarshal(e.RawData, &guildRole); err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild role create payload")
		return
	}

	guild, err := m.getGuild(guildRole.GuildID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_ROLE_CREATE referenced unknown guild %s", guildRole.GuildID)
	}

	// Add the role id to the guild's role ID list then save the role.
	// We will still add the role id to the guild struct as we could fetch
	// it later on and not have the state go stale.
	if ma, err := msgpack.Marshal(guildRole.Role); err == nil {
		if err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guild.ID),
			guildRole.Role.ID,
			ma,
		).Err(); err != nil {
			m.log.Error().Err(err).Msg("failed to add role to redis")
		}
	} else {
		m.log.Error().Err(err).Msg("failed to marshal role")
	}

	guild.Roles = append(guild.Roles, guildRole.Role.ID)
	if err = guild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to save role id to guild on redis")
	}

	ok = true
	se = StreamEvent{
		Type: "GUILD_ROLE_UPDATE",
		Data: &guildRole,
	}

	return
}

func guildRoleDeleteMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error

	// The GuildRoleDelete struct contains the role and guild id
	guildRole := GuildRoleDelete{}
	err = json.Unmarshal(e.RawData, &guildRole)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild role create payload")
		return
	}

	guild, err := m.getGuild(guildRole.GuildID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_ROLE_DELETE referenced unknown guild %s", guildRole.GuildID)
	}

	// Retrieve origional role data to pass to StreamEvent
	role, err := m.getRole(guildRole.GuildID, guildRole.RoleID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_ROLE_DELETE referenced unknown role %s for guild %s", guildRole.RoleID, guildRole.GuildID)
	}

	if err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guild.ID),
		guildRole.RoleID,
	).Err(); err != nil {
		zlog.Error().Err(err).Msg("failed to remove role")
	}

	// We have to construct a new array of strings to remove the guild that was deleted
	// #justGoThings
	newRoles := make([]string, 0)
	for _, id := range guild.Roles {
		if id != guildRole.RoleID {
			newRoles = append(newRoles, id)
		}
	}

	guild.Roles = newRoles
	if err := guild.Save(m); err != nil {
		zlog.Error().Err(err).Msg("failed to save guild on redis")
	}

	ok = true
	se = StreamEvent{
		Type: "GUILD_ROLE_DELETE",
		Data: struct {
			Role    *Role
			GuildID string
		}{
			&role,
			guild.ID,
		},
	}

	return
}

func guildRoleUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error

	guildRole := GuildRoleCreate{}
	err = json.Unmarshal(e.RawData, &guildRole)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild role create payload")
		return
	}

	role, err := m.getRole(guildRole.GuildID, guildRole.Role.ID)
	if err != nil {
		zlog.Error().Err(err).Msgf("GUILD_ROLE_UPDATE referenced unknown role %s in guild %s", guildRole.Role.ID, guildRole.GuildID)
	}

	roleData, err := msgpack.Marshal(guildRole.Role)
	if err != nil {
		m.log.Error().Err(err).Msg("failed to marshal role")
	}
	if err = m.Configuration.redisClient.HSet(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guildRole.GuildID),
		guildRole.Role.ID,
		roleData,
	).Err(); err != nil {
		m.log.Error().Err(err).Msg("failed to set roles")
	}

	ok = true
	se = StreamEvent{
		Type: "ROLE_UPDATE",
		Data: struct {
			Before *Role `msgpack:"before"`
			After  *Role `msgpack:"after"`
		}{
			Before: &role,
			After:  guildRole.Role,
		},
	}

	return
}

func guildChannelCreateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error

	guildChannel := Channel{}
	if err = json.Unmarshal(e.RawData, &guildChannel); err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild channel create payload")
		return
	}

	// Get the guild object to add the channel ID
	guild, err := m.getGuild(guildChannel.GuildID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_CHANNEL_CREATE referenced unknown guild %s", guildChannel.GuildID)
	}

	// Marshal the channel then add it to redis if successful
	if ma, err := msgpack.Marshal(guildChannel); err == nil {
		if err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
			guildChannel.ID,
			ma,
		).Err(); err != nil {
			m.log.Error().Err(err).Msg("failed to add channel")
		}
	} else {
		m.log.Error().Err(err).Msg("failed to marshal channel")
	}

	guild.Channels = append(guild.Channels, guildChannel.ID)
	if err = guild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to save channel id to guild on redis")
	}

	ok = true
	se = StreamEvent{
		Type: "GUILD_CHANNEL_CREATE",
		Data: &guildChannel,
	}

	return
}

// func customEventMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
// }

func init() {
	addMarshaler("READY", readyMarshaler)

	addMarshaler("SHARD_READY", shardReadyMarshaler)
	addMarshaler("SHARD_DISCONNECT", shardDisconnectMarshaler)

	addMarshaler("GUILD_CREATE", guildCreateMarshaler)
	addMarshaler("GUILD_UPDATE", guildUpdateMarshaler)
	addMarshaler("GUILD_DELETE", guildDeleteMarshaler)

	addMarshaler("GUILD_ROLE_CREATE", guildRoleCreateMarshaler)
	addMarshaler("GUILD_ROLE_DELETE", guildRoleDeleteMarshaler)
	addMarshaler("GUILD_ROLE_UPDATE", guildRoleUpdateMarshaler)

	addMarshaler("CHANNEL_CREATE", guildChannelCreateMarshaler)
	// CHANNEL_UPDATE
	// CHANNEL_DELETE
	// CHANNEL_PINS_UPDATE

	// GUILD_MEMBER_ADD
	// GUILD_MEMBER_REMOVE
	// GUILD_MEMBER_UPDATE

	// GUILD_BAN_ADD
	// GUILD_BAN_REMOVE

	// GUILD_EMOJIS_UPDATE

	// GUILD_INTEGRATIONS_UPDATE

	// WEBHOOKS_UPDATE

	// INVITE_CREATE
	// INVITE_DELETE

	// VOICE_STATE_UPDATE

	// PRESENCE_UPDATE

	// MESSAGE_CREATE
	// MESSAGE_UPDATE
	// MESSAGE_DELETE
	// MESSAGE_DELETE_BULK

	// MESSAGE_REACTION_ADD
	// MESSAGE_REACTION_REMOVE
	// MESSAGE_REACTION_REMOVE_ALL
	// MESSAGE_REACTION_REMOVE_EMOJI

	// TYPING START
}
