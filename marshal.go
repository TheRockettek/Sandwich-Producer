package main

import (
	"encoding/json"
	"fmt"
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

	err = guild.Save(m)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to save guild")
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
	if err != nil {
		zlog.Error().Err(err).Msg("failed to check for guild in cache")
	}

	// Check if guild was previously unavailable so we can differentiate
	// between if the bot was just invited or not
	if un, uo := m.Unavailables[guild.ID]; un || ic {
		// If the value was true, this means that during the bot running
		// the guild had previously gone down and is now available again.
		if uo {
			ok = true
			se = StreamEvent{
				Type: "GUILD_AVAILABLE",
				Data: guild,
			}
		} else {
			ok = false
		}
	} else {
		// We will only fire events if they have invited bot
		ok = true
		se = StreamEvent{
			Type: "GUILD_JOIN",
			Data: guild,
		}
	}

	return ok, se
}

func guildDeleteMarshaler(m *Manager, e Event) (ok bool, se StreamEvent) {
	var err error
	var rawData string

	println(string(e.RawData))

	partialGuild := UnavailableGuild{}
	err = json.Unmarshal(e.RawData, &partialGuild)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal partial guild")
		return
	}

	if rawData, err = m.Configuration.redisClient.HGet(
		ctx,
		fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
		partialGuild.ID,
	).Result(); err != nil {
		zlog.Error().Err(err).Msg("failed to remove guild")
	}

	guild := MarshalGuild{}
	guild.From([]byte(rawData))

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
			Data: guild,
		}
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
	// GUILD_UPDATE
	addMarshaler("GUILD_DELETE", guildDeleteMarshaler)

	// GUILD_ROLE_CREATE
	// GUILD_ROLE_DELETE
	// GUILD_ROLE_UPDATE

	// CHANNEL_CREATE
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
