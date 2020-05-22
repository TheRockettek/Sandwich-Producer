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
		m.Unavailables[g.ID] = true
	}

	return
}

func guildCreateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error
	var ma []byte

	guild := MarshalGuild{}
	err = json.Unmarshal(e.RawData, &guild)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild")
		return
	}

	// Create snowflakes from role, channel and emoji objects

	guildRoles := make(map[string]interface{})
	guild.Roles = make([]string, 0)
	for _, r := range guild.RoleValues {
		guild.Roles = append(guild.Roles, r.ID)
		if ma, err = msgpack.Marshal(r); err == nil {
			guildRoles[r.ID] = ma
		} else {
			zlog.Error().Err(err).Msg("failed to marshal role")
		}
	}

	guildChannels := make(map[string]interface{})
	guild.Channels = make([]string, 0)
	for _, c := range guild.ChannelValues {
		guild.Channels = append(guild.Channels, c.ID)
		if ma, err = msgpack.Marshal(c); err == nil {
			guildChannels[c.ID] = ma
		} else {
			zlog.Error().Err(err).Msg("failed to marshal channel")
		}
	}

	guildEmojis := make(map[string]interface{})
	guild.Emojis = make([]string, 0)
	for _, e := range guild.EmojiValues {
		guild.Emojis = append(guild.Emojis, e.ID)
		if ma, err = msgpack.Marshal(e); err == nil {
			guildEmojis[e.ID] = ma
		} else {
			zlog.Error().Err(err).Msg("failed to marshal emoji")
		}
	}

	if err = m.Configuration.redisClient.HMSet(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guild.ID),
		guildRoles,
	).Err(); err != nil {
		zlog.Error().Err(err).Msg("failed to set roles")
	}

	if err = m.Configuration.redisClient.HMSet(
		ctx,
		fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
		guildChannels,
	).Err(); err != nil {
		zlog.Error().Err(err).Msg("failed to set channels")
	}

	if err = m.Configuration.redisClient.HMSet(
		ctx,
		fmt.Sprintf("%s:emojis", m.Configuration.RedisPrefix),
		guildEmojis,
	).Err(); err != nil {
		zlog.Error().Err(err).Msg("failed to set emojis")
	}

	if ma, err = msgpack.Marshal(guild); err != nil {
		if err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:guild", m.Configuration.RedisPrefix),
			guild.ID,
			ma,
		).Err(); err != nil {
			zlog.Error().Err(err).Msg("failed to set guild")
		}
	} else {
		zlog.Error().Err(err).Msg("failed to marshal guild")
	}

	// TODO: Add struct information to redis

	// Check if guild was previously unavailable so we can differentiate
	// between if the bot was just invited or not
	if un, uo := m.Unavailables[guild.ID]; un && uo {
		ok = false
	} else {
		println("Guild invited bot")
		// We will only fire events if they have invited bot
		ok = true
		se = StreamEvent{
			Type: "GUILD_JOIN",
			Data: guild,
		}
	}
	// Remove guild from unavailables as we do not need it anymore
	delete(m.Unavailables, guild.ID)

	return ok, se
}

func init() {
	addMarshaler("READY", readyMarshaler)

	addMarshaler("SHARD_READY", shardReadyMarshaler)
	addMarshaler("SHARD_DISCONNECT", shardDisconnectMarshaler)

	addMarshaler("GUILD_CREATE", guildCreateMarshaler)
	// GUILD_DELETE
	// GUILD_UPDATE

	// GUILD_EMOJIS_UPDATE
	// GUILD_INTEGRATIONS_UPDATE

	// GUILD_BAN_ADD
	// GUILD_BAN_REMOVE

	// GUILD_MEMBER_ADD
	// GUILD_MEMBER_REMOVE
	// GUILD_MEMBER_UPDATE

	// GUILD_ROLE_CREATE
	// GUILD_ROLE_DELETE
	// GUILD_ROLE_UPDATE

	// MESSAGE_CREATE
	// MESSAGE_DELETE
	// MESSAGE_DELETE_BULK
	// MESSAGE_UPDATE

	// MESSAGE_REACTION_ADD
	// MESSAGE_REACTION_REMOVE
	// MESSAGE_REACTION_REMOVE_ALL

}
