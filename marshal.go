package main

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/vmihailenco/msgpack"
)

var marshalers = make(map[string]func(*Manager, Event) (bool, StreamEvent, error))

// Marshalers are used to both handle state caching and also produce a structure that is sent to consumers.
// The boolean in a marshaler dictates if that event should be directed to consumers for manual overrides.
// This only manually overrides blocking so if it returns true and it is in the blacklist, it will still be
// sent to consumers.

// addMarshaler adds a marshaler for a specific event.
func addMarshaler(event string, marshaler func(*Manager, Event) (bool, StreamEvent, error)) {

	if _, ok := marshalers[event]; ok {
		return
	}
	marshalers[event] = marshaler
}

func shardReadyMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	ok, se.Data = true, e.Data
	m.log.Info().Msgf("Shard %d is ready", e.Data.(struct {
		ShardID int `msgpack:"shard_id"`
	}).ShardID)
	return
}

func shardConnectMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	ok, se.Data = true, e.Data
	m.log.Info().Msgf("Shard %d has connected", se.Data.(struct {
		ShardID int `msgpack:"shard_id"`
	}).ShardID)
	return
}

func shardDisconnectMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	ok, se.Data = true, e.Data
	m.log.Info().Msgf("Shard %d has disconnected with code %d", se.Data.(ShardDisconnectOp).ShardID, se.Data.(ShardDisconnectOp).StatusCode)
	return
}

func readyMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	ready := Ready{}
	err = jsoniter.Unmarshal(e.RawData, &ready)
	if err != nil {
		return
	}

	g := ready.Guilds[0]
	guildID, _ := strconv.Atoi(g.ID)
	shardID := (guildID >> 22) % m.Configuration.ShardCount
	counter := m.UnavailableCounters[shardID]

	// Add the specific shards guilds to LockSet that is for the manager
	m.unavailablesMu.Lock()
	for _, g = range ready.Guilds {
		m.Unavailables[g.ID] = false
		counter.Add(g.ID)
	}
	m.unavailablesMu.Unlock()

	ok = true

	return
}

func guildCreateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	guild := MarshalGuild{}
	err = guild.Create(e.RawData)
	if err != nil {
		return
	}

	guildID, _ := strconv.Atoi(guild.ID)
	shardID := (guildID >> 22) % m.Configuration.ShardCount
	counter := m.UnavailableCounters[shardID]
	counter.Remove(guild.ID)

	// If the LockSet is now empty, we know all unavailables were added so it is
	// fully ready now.
	println(counter.Len())
	if counter.Len() <= 0 {
		m.Sessions[shardID].OnEvent(Event{
			Type: "SHARD_READY",
			Data: struct {
				ShardID int `msgpack:"shard_id"`
			}{
				ShardID: shardID,
			},
		})
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
		m.log.Error().Err(err).Msg("Failed to store guild")
		err = nil
	}

	// Check if guild is in the pending availability map or is currently in cache.
	m.unavailablesMu.RLock()
	defer m.unavailablesMu.RUnlock()
	if un, uo := m.Unavailables[guild.ID]; uo || ic {
		// If the guild was unavailable, this means it is now available so fire the
		// available event. If it is in the cache it also means that it is likely to
		// be available again incase we did not get the unavailable payload. If neither,
		// this just means it was initial GUILD_CREATE event when the bot is connecting
		// and we can just ignore it.

		// If was unavailable and is in cache, it was down so fire guild available
		if un && ic {
			ok = true
			se = StreamEvent{
				Type: "GUILD_AVAILABLE",
				Data: &guild,
			}
		}
		// If not unavailable and not in cache, it is initial guild create
		if !(un && ic) {
			if m.Configuration.Features.CacheMembers {
				MemberMarshals := make(map[string]interface{})
				for _, me := range guild.Members {
					me.GuildID = guild.ID
					ma, err := me.Marshaled(true, m)
					MemberMarshals[me.ID] = ma
					if err != nil {
						m.log.Error().Err(err).Msgf("Failed to marshal member '%s' struct", me.ID)
						err = nil
					}
				}

				err = m.Configuration.redisClient.HSet(
					ctx,
					fmt.Sprintf("%s:guild:%s:members", m.Configuration.RedisPrefix, guild.ID),
					MemberMarshals,
				).Err()
				if err != nil {
					m.log.Error().Err(err).Msg("Failed to store members")
					err = nil
				}

				m.log.Trace().Msgf("Added %d member(s) to state for guild %s", len(guild.Members), guild.ID)
			}
		}

		// If it was in unavailables and either wasnt marked unavailable or was in cache,
		// it is probrably just a shard that reconnected so dont bother
		ok = false
	} else {
		// We will only fire events if they have invited bot
		ok = true
		se = StreamEvent{
			Type: "GUILD_JOIN",
			Data: &guild,
		}
	}

	return
}

func guildDeleteMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	partialGuild := UnavailableGuild{}
	err = jsoniter.Unmarshal(e.RawData, &partialGuild)
	if err != nil {
		m.log.Error().Err(err).Msg("Failed to unmarshal UnavailableGuild struct")
		err = nil
		return
	}

	m.unavailablesMu.Lock()
	delete(m.Unavailables, partialGuild.ID)
	m.unavailablesMu.Unlock()

	if partialGuild.Unavailable {
		// guild has gone down
		m.unavailablesMu.Lock()
		m.Unavailables[partialGuild.ID] = true
		m.unavailablesMu.Unlock()
		ok = true
		se = StreamEvent{
			Type: "GUILD_UNAVAILABLE",
			Data: partialGuild,
		}
	} else {
		// user has left guild
		if err = partialGuild.Delete(m); err != nil {
			m.log.Error().Err(err).Msg("Failed to delete guild")
			err = nil
		}

		ok = true
		se = StreamEvent{
			Type: "GUILD_REMOVE",
			Data: &partialGuild,
		}
	}

	return
}

func guildUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	updatedGuild := MarshalGuild{}
	err = updatedGuild.Create(e.RawData)
	if err != nil {
		m.log.Error().Err(err).Msg("Failed to create MarshalGuild")
		err = nil
		return
	}

	guild, err := m.getGuild(updatedGuild.ID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_UPDATE referenced unknown guild '%s'", updatedGuild.ID)
		err = nil
	}

	if err = updatedGuild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("Failed to update Guild")
		err = nil
	}

	if !reflect.DeepEqual(&guild, &updatedGuild) {
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
	} else {
		// m.log.Debug().Msg("not creating GUILD_UPDATE as the before and after seem equal")
		ok = false
	}

	return
}

func guildRoleCreateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	// The GuildRoleCreate struct contains the role and guild id
	guildRole := GuildRoleCreate{}
	if err = jsoniter.Unmarshal(e.RawData, &guildRole); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_ROLE_CREATE payload")
		return
	}

	guild, err := m.getGuild(guildRole.GuildID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_ROLE_CREATE referenced unknown guild %s", guildRole.GuildID)
	}

	// Add the role id to the guild's role ID list then save the role.
	// We will still add the role id to the guild struct as we could fetch
	// it later on and not have the state go stale.
	if err := guildRole.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to add role to redis")
	}

	guild.Roles = append(guild.Roles, guildRole.Role.ID)
	if err = guild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to save role id to guild on redis")
	}

	ok = true
	se = StreamEvent{
		Type: "GUILD_ROLE_CREATE",
		Data: &guildRole,
	}

	return
}

func guildRoleDeleteMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	// The GuildRole struct contains the role and guild id
	guildRole := GuildRoleDelete{}
	err = jsoniter.Unmarshal(e.RawData, &guildRole)
	if err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_ROLE_DELETE payload")
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

	if err = guildRole.Delete(m); err != nil {
		m.log.Error().Err(err).Msg("failed to remove role")
	}

	// We have to construct a new array of strings to remove the role that was deleted
	newRoles := make([]string, 0)
	for _, id := range guild.Roles {
		if id != guildRole.RoleID {
			newRoles = append(newRoles, id)
		}
	}

	guild.Roles = newRoles
	if err := guild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to save guild on redis")
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

func guildRoleUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	guildRole := GuildRoleCreate{}
	err = jsoniter.Unmarshal(e.RawData, &guildRole)
	if err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_ROLE_UPDATE payload")
		return
	}

	role, err := m.getRole(guildRole.GuildID, guildRole.Role.ID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_ROLE_UPDATE referenced unknown role %s in guild %s", guildRole.Role.ID, guildRole.GuildID)
	}

	if err = guildRole.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to add role to redis")
	}

	if !reflect.DeepEqual(&role, guildRole.Role) {
		ok = true
		se = StreamEvent{
			Type: "GUILD_ROLE_UPDATE",
			Data: struct {
				Before *Role `msgpack:"before"`
				After  *Role `msgpack:"after"`
			}{
				Before: &role,
				After:  guildRole.Role,
			},
		}
	} else {
		// m.log.Debug().Msg("not creating ROLE_UPDATE as the before and after seem equal")
		ok = false
	}

	return
}

func guildChannelCreateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	guildChannel := Channel{}
	if err = jsoniter.Unmarshal(e.RawData, &guildChannel); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal guild CHANNEL_CREATE payload")
		return
	}

	// Get the guild object to add the channel ID
	guild, err := m.getGuild(guildChannel.GuildID)
	if err != nil {
		m.log.Error().Err(err).Msgf("CHANNEL_CREATE referenced unknown guild %s", guildChannel.GuildID)
	}

	// Marshal the channel then add it to redis if successful
	if err := guildChannel.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to add channel to redis")
	}

	guild.Channels = append(guild.Channels, guildChannel.ID)
	if err = guild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to save channel id to guild on redis")
	}

	ok = true
	se = StreamEvent{
		Type: "CHANNEL_CREATE",
		Data: &guildChannel,
	}

	return
}

func guildChannelUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	updatedChannel := Channel{}
	if err = jsoniter.Unmarshal(e.RawData, &updatedChannel); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal CHANNEL_UPDATE payload")
		return
	}

	channel, err := m.getChannel(updatedChannel.ID)
	if err != nil {
		m.log.Error().Err(err).Msgf("CHANNEL_UPDATE referenced unknown channel %s in guild %s", updatedChannel.ID, updatedChannel.GuildID)
	}

	if err = updatedChannel.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to update guild")
	}

	if !reflect.DeepEqual(&channel, &updatedChannel) {
		ok = true
		se = StreamEvent{
			Type: "CHANNEL_UPDATE",
			Data: struct {
				Before *Channel `msgpack:"before"`
				After  *Channel `msgpack:"after"`
			}{
				Before: &channel,
				After:  &updatedChannel,
			},
		}
	} else {
		// m.log.Debug().Msg("not creating CHANNEL_UPDATE as the before and after seem equal")
		ok = false
	}

	return
}

func guildChannelDeleteMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	guildChannel := Channel{}
	if err = jsoniter.Unmarshal(e.RawData, &guildChannel); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal CHANNEL_DELETE payload")
		return
	}

	guild, err := m.getGuild(guildChannel.GuildID)
	if err != nil {
		m.log.Error().Err(err).Msgf("CHANNEL_DELETE referenced unknown guild %s", guildChannel.GuildID)
	}

	if err = guildChannel.Delete(m); err != nil {
		m.log.Error().Err(err).Msg("failed to remove channel")
	}

	// We have to construct a new array of strings to remove the channel that was deleted
	newChannels := make([]string, 0)
	for _, id := range guild.Channels {
		if id != guildChannel.ID {
			newChannels = append(newChannels, id)
		}
	}

	guild.Channels = newChannels
	if err := guild.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to save guild on redis")
	}

	ok = true
	se = StreamEvent{
		Type: "CHANNEL_DELETE",
		Data: &guildChannel,
	}

	return
}

func guildChannelPinsUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	pinsPayload := ChannelPinsUpdate{}
	if err = jsoniter.Unmarshal(e.RawData, &pinsPayload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal CHANNEL_PINS_UPDATE payload")
		return
	}

	channel, err := m.getChannel(pinsPayload.ChannelID)
	if err != nil {
		m.log.Error().Err(err).Msgf("CHANNEL_PINS_UPDATE referenced unknown channel %s in guild %s", pinsPayload.ChannelID, pinsPayload.GuildID)
	}

	channel.LastPinTimestamp = Timestamp(pinsPayload.LastPinTimestamp)

	if err = channel.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to update guild")
	}

	ok = true
	se = StreamEvent{
		Type: "CHANNEL_PINS_UPDATE",
		Data: &pinsPayload,
	}

	return
}

func guildMemberAddMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {
	memberPayload := Member{}
	if err = jsoniter.Unmarshal(e.RawData, &memberPayload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_MEMBER_ADD payload")
		return
	}

	if m.Configuration.Features.CacheMembers {
		ma, _ := memberPayload.Marshaled(true, m)
		err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:guild:%s:members", m.Configuration.RedisPrefix, memberPayload.GuildID),
			memberPayload.ID,
			ma,
		).Err()
	}

	ok = true
	se = StreamEvent{
		Type: "GUILD_MEMBER_ADD",
		Data: &memberPayload,
	}

	return
}

func guildMemberRemoveMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	memberPayload := Member{}
	if err = jsoniter.Unmarshal(e.RawData, &memberPayload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_MEMBER_REMOVE payload")
	}

	err = memberPayload.Delete(m)

	ok = true
	se = StreamEvent{
		Type: "GUILD_MEMBER_REMOVE",
		Data: &memberPayload,
	}

	return
}

func guildMemberUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	updatedMember := Member{}
	if err = jsoniter.Unmarshal(e.RawData, &updatedMember); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_MEMBER_UPDATE payload")
		return
	}

	updatedMember.ID = updatedMember.User.ID
	member, err := m.getMember(updatedMember.GuildID, updatedMember.ID)
	if err != nil {
		m.log.Error().Err(err).Msgf("GUILD_MEMBER_UPDATE referenced unknown member %s in guild %s", updatedMember.ID, updatedMember.GuildID)
		return
	}

	// If the member we get includes a user, inherit the mutual guilds. We could just
	// directly copy the entire user object but incase the user object changes for some
	// reason we might aswell use the most recent version. We can't directly reference
	// the Mutual attribute of the origional member as it has a mutex which cannot be
	// cloned so we play it safe and make a completely new object.
	if member.UserIncluded {
		updatedMember.UserIncluded = true
		updatedMember.User.Mutual = MutualGuilds{
			Guilds: LockSet{
				Values: member.User.Mutual.Guilds.Values,
			},
			Removed: LockSet{
				Values: member.User.Mutual.Removed.Values,
			},
		}
	}

	if err = updatedMember.Save(m); err != nil {
		m.log.Error().Err(err).Msg("failed to update member")
	}

	if !reflect.DeepEqual(&member, &updatedMember) {
		ok = true
		se = StreamEvent{
			Type: "GUILD_MEMBER_UPDATE",
			Data: struct {
				Before *Member `msgpack:"before"`
				After  *Member `msgpack:"after"`
			}{
				Before: &member,
				After:  &updatedMember,
			},
		}
	} else {
		ok = false
	}

	return
}

func guildMembersChunkMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {
	// This marshaler simply handles guild members chunks and will not forward the data
	// to consumers as it is simply for states

	chunkPayload := GuildMembersChunk{}
	if err = jsoniter.Unmarshal(e.RawData, &chunkPayload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_MEMBERS_CHUNK payload")
		return
	}

	guild, err := m.getGuild(chunkPayload.GuildID)
	if err != nil {
		return
	}

	// Really should rewrite this to not marshal the user object if we are
	// not caching members... We still have this as Marshaled(true, m) will
	// add the mutual guild still.
	MemberMarshals := make(map[string]interface{})
	for _, me := range chunkPayload.Members {
		me.GuildID = guild.ID
		ma, err := me.Marshaled(true, m)
		MemberMarshals[me.ID] = ma
		if err != nil {
			m.log.Error().Err(err).Msg("failed to marshal member")
		}
	}

	if m.Configuration.Features.CacheMembers {
		err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:guild:%s:members", m.Configuration.RedisPrefix, guild.ID),
			MemberMarshals,
		).Err()
		if err != nil {
			m.log.Error().Err(err).Msg("failed to add members to state")
		}
	}

	m.log.Trace().Msgf("Added %d members from chunk for guild %s", len(chunkPayload.Members), guild.ID)

	return
}

func guildBanAddMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	payload := GuildBanAdd{}
	if err = jsoniter.Unmarshal(e.RawData, &payload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_BAN_ADD payload")
		return
	}

	ok = true
	se = StreamEvent{
		Type: e.Type,
		Data: &payload,
	}

	return
}

func guildBanRemoveMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	payload := GuildBanRemove{}
	if err = jsoniter.Unmarshal(e.RawData, &payload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_BAN_REMOVE payload")
		return
	}

	ok = true
	se = StreamEvent{
		Type: e.Type,
		Data: &payload,
	}

	return
}

func guildEmojisUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	payload := GuildEmojisUpdate{}
	if err = jsoniter.Unmarshal(e.RawData, &payload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_EMOJIS_UPDATE payload")
		return
	}

	g, err := m.getGuild(payload.GuildID)

	g.Emojis = make([]string, 0)
	guildEmojis := make(map[string]interface{})
	for _, e := range payload.Emojis {
		g.Emojis = append(g.Emojis, e.ID)
		if ma, err := msgpack.Marshal(e); err == nil {
			guildEmojis[e.ID] = ma
		}
	}

	if len(guildEmojis) > 0 {
		err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:emojis", m.Configuration.RedisPrefix),
			guildEmojis,
		).Err()
	}

	if ma, err := msgpack.Marshal(g); err == nil {
		err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
			g.ID,
			ma,
		).Err()
	}

	ok = true
	se = StreamEvent{
		Type: e.Type,
		Data: &payload,
	}

	return
}

func guildIntegrationsUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {
	// What is even the point of this event...

	payload := GuildIntegrationsUpdate{}
	if err = jsoniter.Unmarshal(e.RawData, &payload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal GUILD_INTEGRATIONS_UPDATE payload")
		return
	}

	ok = true
	se = StreamEvent{
		Type: e.Type,
		Data: &payload,
	}

	return
}

func webhooksUpdateMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {

	payload := WebhooksUpdate{}
	if err = jsoniter.Unmarshal(e.RawData, &payload); err != nil {
		m.log.Error().Err(err).Msg("failed to unmarshal WEBHOOKS_UPDATE payload")
		return
	}

	ok = true
	se = StreamEvent{
		Type: e.Type,
		Data: &payload,
	}

	return
}

// func customEventMarshaler(m *Manager, e Event) (ok bool, se StreamEvent, err error) {
// }

func init() {
	addMarshaler("READY", readyMarshaler)

	addMarshaler("SHARD_READY", shardReadyMarshaler)
	addMarshaler("SHARD_CONNECT", shardConnectMarshaler)
	addMarshaler("SHARD_DISCONNECT", shardDisconnectMarshaler)

	addMarshaler("GUILD_CREATE", guildCreateMarshaler)
	addMarshaler("GUILD_UPDATE", guildUpdateMarshaler)
	addMarshaler("GUILD_DELETE", guildDeleteMarshaler)

	addMarshaler("GUILD_ROLE_CREATE", guildRoleCreateMarshaler)
	addMarshaler("GUILD_ROLE_DELETE", guildRoleDeleteMarshaler)
	addMarshaler("GUILD_ROLE_UPDATE", guildRoleUpdateMarshaler)

	addMarshaler("CHANNEL_CREATE", guildChannelCreateMarshaler)
	addMarshaler("CHANNEL_UPDATE", guildChannelUpdateMarshaler)
	addMarshaler("CHANNEL_DELETE", guildChannelDeleteMarshaler)
	addMarshaler("CHANNEL_PINS_UPDATE", guildChannelPinsUpdateMarshaler)

	addMarshaler("GUILD_MEMBER_ADD", guildMemberAddMarshaler)
	addMarshaler("GUILD_MEMBER_REMOVE", guildMemberRemoveMarshaler)
	addMarshaler("GUILD_MEMBER_UPDATE", guildMemberUpdateMarshaler)
	addMarshaler("GUILD_MEMBERS_CHUNK", guildMembersChunkMarshaler)

	addMarshaler("GUILD_BAN_ADD", guildBanAddMarshaler)
	addMarshaler("GUILD_BAN_REMOVE", guildBanRemoveMarshaler)

	addMarshaler("GUILD_EMOJIS_UPDATE", guildEmojisUpdateMarshaler)

	addMarshaler("GUILD_INTEGRATIONS_UPDATE", guildIntegrationsUpdateMarshaler)

	addMarshaler("WEBHOOKS_UPDATE", webhooksUpdateMarshaler)

	// INVITE_CREATE
	// INVITE_DELETE

	// VOICE_STATE_UPDATE

	// PRESENCE_UPDATE
	// USER_UPDATE

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
