package main

import (
	"fmt"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// StreamEvent provides the struct for events that are sent over STAN/NATS
type StreamEvent struct {
	Type string      `msgpack:"i"`
	Data interface{} `msgpack:"d"`
}

// Event provides a basic initial struct for all websocket events.
type Event struct {
	Operation int                 `json:"op" msgpack:"op"`
	Sequence  int64               `json:"s" msgpack:"s"`
	Type      string              `json:"t" msgpack:"t"`
	RawData   jsoniter.RawMessage `json:"d" msgpack:"-"`

	// Marshalled event
	Data interface{} `json:"-" msgpack:"d"`
}

// RequestGuildMembers is the data sent to discord when requesting guild members.
type RequestGuildMembers struct {
	Op   int `json:"op" msgpack:"op"`
	Data struct {
		GuildID string `json:"guild_id" msgpack:"guild_id"`
		Query   string `json:"query" msgpack:"query"`
		Limit   int    `json:"limit" msgpack:"limit"`
	} `json:"d" msgpack:"d"`
}

// Resume is the packet that we send to discord.
type Resume struct {
	Op   int `json:"op" msgpack:"op"`
	Data struct {
		Token     string `json:"token" msgpack:"token"`
		SessionID string `json:"session_id" msgpack:"session_id"`
		Sequence  int64  `json:"seq" msgpack:"seq"`
	} `json:"d" msgpack:"d"`
}

// Hello is the data sent for the Hello event.
type Hello struct {
	HeartbeatInterval time.Duration `json:"heartbeat_interval" msgpack:"heartbeat_interval"`
}

// Heartbeat is the data for the Heartbeat event.
type Heartbeat struct {
	Op   int   `json:"op" msgpack:"op"`
	Data int64 `json:"d" msgpack:"d"`
}

// UpdateStatus is the data send to update the status.
type UpdateStatus struct {
	Op   int              `json:"op" msgpack:"op"`
	Data UpdateStatusData `json:"d" msgpack:"d"`
}

// A Ready stores all data for the websocket READY event.
type Ready struct {
	Version         int        `json:"v" msgpack:"v"`
	SessionID       string     `json:"session_id" msgpack:"session_id"`
	User            *User      `json:"user" msgpack:"user"`
	PrivateChannels []*Channel `json:"private_channels" msgpack:"private_channels"`
	Guilds          []*Guild   `json:"guilds" msgpack:"guilds"`
}

// Identify is the data sent when identifying
type Identify struct {
	Op   int          `json:"op" msgpack:"op"`
	Data identifyData `json:"d" msgpack:"d"`
}

type identifyProperties struct {
	OS      string `json:"$os" msgpack:"$os"`
	Browser string `json:"$browser" msgpack:"$browser"`
	Device  string `json:"$device" msgpack:"$device"`
}

type identifyData struct {
	Token          string             `json:"token" msgpack:"token"`
	Properties     identifyProperties `json:"properties" msgpack:"properties"`
	LargeThreshold int                `json:"large_threshold" msgpack:"large_threshold"`
	Compress       bool               `json:"compress" msgpack:"compress"`
	Shard          *[2]int            `json:"shard,omitempty" msgpack:"shard,omitempty"`
	Presence       UpdateStatusData   `json:"presence,omitempty" msgpack:"presence,omitempty"`
}

// ChannelCreate is the data for a ChannelCreate event.
type ChannelCreate struct {
	*Channel
}

// ChannelUpdate is the data for a ChannelUpdate event.
type ChannelUpdate struct {
	*Channel
}

// ChannelDelete is the data for a ChannelDelete event.
type ChannelDelete struct {
	*Channel
}

// ChannelPinsUpdate stores data for a ChannelPinsUpdate event.
type ChannelPinsUpdate struct {
	LastPinTimestamp string `json:"last_pin_timestamp" msgpack:"last_pin_timestamp"`
	ChannelID        string `json:"channel_id" msgpack:"channel_id"`
	GuildID          string `json:"guild_id,omitempty" msgpack:"guild_id,omitempty"`
}

// GuildCreate is the data for a GuildCreate event.
type GuildCreate struct {
	*Guild
}

// GuildUpdate is the data for a GuildUpdate event.
type GuildUpdate struct {
	*Guild
}

// GuildDelete is the data for a GuildDelete event.
type GuildDelete struct {
	*Guild
}

// GuildBanAdd is the data for a GuildBanAdd event.
type GuildBanAdd struct {
	User    *User  `json:"user" msgpack:"user"`
	GuildID string `json:"guild_id" msgpack:"guild_id"`
}

// GuildBanRemove is the data for a GuildBanRemove event.
type GuildBanRemove struct {
	User    *User  `json:"user" msgpack:"user"`
	GuildID string `json:"guild_id" msgpack:"guild_id"`
}

// GuildMemberAdd is the data for a GuildMemberAdd event.
type GuildMemberAdd struct {
	*Member
}

// GuildMemberUpdate is the data for a GuildMemberUpdate event.
type GuildMemberUpdate struct {
	*Member
}

// GuildMemberRemove is the data for a GuildMemberRemove event.
type GuildMemberRemove struct {
	*Member
}

// GuildRoleCreate is the data for a GuildRoleCreate event.
type GuildRoleCreate struct {
	*GuildRole
}

// GuildRoleUpdate is the data for a GuildRoleUpdate event.
type GuildRoleUpdate struct {
	*GuildRole
}

// A GuildRoleDelete is the data for a GuildRoleDelete event.
type GuildRoleDelete struct {
	RoleID  string `json:"role_id" msgpack:"role_id"`
	GuildID string `json:"guild_id" msgpack:"guild_id"`
}

// Delete removes the role from redis
func (grd *GuildRoleDelete) Delete(m *Manager) (err error) {
	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, grd.GuildID),
		grd.RoleID,
	).Err()

	return
}

// A GuildEmojisUpdate is the data for a guild emoji update event.
type GuildEmojisUpdate struct {
	GuildID string   `json:"guild_id" msgpack:"guild_id"`
	Emojis  []*Emoji `json:"emojis" msgpack:"emojis"`
}

// A GuildMembersChunk is the data for a GuildMembersChunk event.
type GuildMembersChunk struct {
	GuildID string    `json:"guild_id" msgpack:"guild_id"`
	Members []*Member `json:"members" msgpack:"members"`
}

// GuildIntegrationsUpdate is the data for a GuildIntegrationsUpdate event.
type GuildIntegrationsUpdate struct {
	GuildID string `json:"guild_id" msgpack:"guild_id"`
}

// InviteCreate is the data for an InviteCreate event.
type InviteCreate struct {
	ChannelID      string        `json:"channel_id" msgpack:"channel_id"`
	GuildID        string        `json:"guild_id,omitempty" msgpack:"guild_id,omitempty"`
	Inviter        *User         `json:"inviter,omitempty" msgpack:"inviter,omitempty"`
	Code           string        `json:"code" msgpack:"code"`
	CreatedAt      Timestamp     `json:"created_at" msgpack:"created_at"`
	MaxAge         time.Duration `json:"max_age" msgpack:"max_age"`
	MaxUses        int           `json:"max_uses" msgpack:"max_uses"`
	TargetUser     *User         `json:"target_user" msgpack:"target_user"`
	TargetUserType int           `json:"target_user_type,omitempty" msgpack:"target_user_type,omitempty"`
	Temporary      bool          `json:"temporary" msgpack:"temporary"`
	Uses           int           `json:"uses" msgpack:"uses"`
}

// InviteDelete is the data for an InviteDelete event.
type InviteDelete struct {
	ChannelID string `json:"channel_id" msgpack:"channel_id"`
	GuildID   string `json:"guild_id,omitempty" msgpack:"guild_id,omitempty"`
	Code      string `json:"code" msgpack:"code"`
}

// MessageAck is the data for a MessageAck event.
type MessageAck struct {
	MessageID string `json:"message_id" msgpack:"message_id"`
	ChannelID string `json:"channel_id" msgpack:"channel_id"`
}

// MessageCreate is the data for a MessageCreate event.
type MessageCreate struct {
	*Message
}

// MessageUpdate is the data for a MessageUpdate event.
type MessageUpdate struct {
	*Message
	// BeforeUpdate will be nil if the Message was not previously cached in the state cache.
	BeforeUpdate *Message `json:"-" msgpack:"-"`
}

// MessageDelete is the data for a MessageDelete event.
type MessageDelete struct {
	*Message
}

// MessageReactionAdd is the data for a MessageReactionAdd event.
type MessageReactionAdd struct {
	*MessageReaction
}

// MessageReactionRemove is the data for a MessageReactionRemove event.
type MessageReactionRemove struct {
	*MessageReaction
}

// MessageReactionRemoveAll is the data for a MessageReactionRemoveAll event.
type MessageReactionRemoveAll struct {
	*MessageReaction
}

// Resumed is the data for a Resumed event.
type Resumed struct {
	Trace []string `json:"_trace" msgpack:"_trace"`
}

// TypingStart is the data for a TypingStart event.
type TypingStart struct {
	UserID    string `json:"user_id" msgpack:"user_id"`
	ChannelID string `json:"channel_id" msgpack:"channel_id"`
	GuildID   string `json:"guild_id,omitempty" msgpack:"guild_id,omitempty"`
	Timestamp int    `json:"timestamp" msgpack:"timestamp"`
}

// UserUpdate is the data for a UserUpdate event.
type UserUpdate struct {
	*User
}

// VoiceServerUpdate is the data for a VoiceServerUpdate event.
type VoiceServerUpdate struct {
	Token    string `json:"token" msgpack:"token"`
	GuildID  string `json:"guild_id" msgpack:"guild_id"`
	Endpoint string `json:"endpoint" msgpack:"endpoint"`
}

// VoiceStateUpdate is the data for a VoiceStateUpdate event.
type VoiceStateUpdate struct {
	*VoiceState
}

// MessageDeleteBulk is the data for a MessageDeleteBulk event
type MessageDeleteBulk struct {
	Messages  []string `json:"ids" msgpack:"ids"`
	ChannelID string   `json:"channel_id" msgpack:"channel_id"`
	GuildID   string   `json:"guild_id" msgpack:"guild_id"`
}

// WebhooksUpdate is the data for a WebhooksUpdate event
type WebhooksUpdate struct {
	GuildID   string `json:"guild_id" msgpack:"guild_id"`
	ChannelID string `json:"channel_id" msgpack:"channel_id"`
}
