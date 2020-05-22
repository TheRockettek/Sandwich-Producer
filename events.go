package main

import (
	"encoding/json"
	"time"
)

// StreamEvent provides the struct for events that are sent over STAN/NATS
type StreamEvent struct {
	Type string      `msgpack:"i"`
	Data interface{} `msgpack:"d"`
}

// Event provides a basic initial struct for all websocket events.
type Event struct {
	Operation int             `json:"op" msgpack:"op"`
	Sequence  int64           `json:"s" msgpack:"s"`
	Type      string          `json:"t" msgpack:"t"`
	RawData   json.RawMessage `json:"d" msgpack:"-"`

	// Marshalled event
	Data interface{} `json:"-" msgpack:"d"`
}

// RequestGuildMembers is the data sent to discord when requesting guild members.
type RequestGuildMembers struct {
	Op   int `json:"op"`
	Data struct {
		GuildID string `json:"guild_id"`
		Query   string `json:"query"`
		Limit   int    `json:"limit"`
	} `json:"d"`
}

// Resume is the packet that we send to discord.
type Resume struct {
	Op   int `json:"op"`
	Data struct {
		Token     string `json:"token"`
		SessionID string `json:"session_id"`
		Sequence  int64  `json:"seq"`
	} `json:"d"`
}

// Hello is the data sent for the Hello event.
type Hello struct {
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

// Heartbeat is the data for the Heartbeat event.
type Heartbeat struct {
	Op   int   `json:"op"`
	Data int64 `json:"d"`
}

// UpdateStatus is the data send to update the status.
type UpdateStatus struct {
	Op   int              `json:"op"`
	Data UpdateStatusData `json:"d"`
}

// A Ready stores all data for the websocket READY event.
type Ready struct {
	Version         int        `json:"v"`
	SessionID       string     `json:"session_id"`
	User            *User      `json:"user"`
	PrivateChannels []*Channel `json:"private_channels"`
	Guilds          []*Guild   `json:"guilds"`
}

// Identify is the data sent when identifying
type Identify struct {
	Op   int          `json:"op"`
	Data identifyData `json:"d"`
}

type identifyProperties struct {
	OS      string `json:"$os"`
	Browser string `json:"$browser"`
	Device  string `json:"$device"`
}

type identifyData struct {
	Token          string             `json:"token"`
	Properties     identifyProperties `json:"properties"`
	LargeThreshold int                `json:"large_threshold"`
	Compress       bool               `json:"compress"`
	Shard          *[2]int            `json:"shard,omitempty"`
	Presence       UpdateStatusData   `json:"presence,omitempty"`
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
	LastPinTimestamp string `json:"last_pin_timestamp"`
	ChannelID        string `json:"channel_id"`
	GuildID          string `json:"guild_id,omitempty"`
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
	User    *User  `json:"user"`
	GuildID string `json:"guild_id"`
}

// GuildBanRemove is the data for a GuildBanRemove event.
type GuildBanRemove struct {
	User    *User  `json:"user"`
	GuildID string `json:"guild_id"`
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
	RoleID  string `json:"role_id"`
	GuildID string `json:"guild_id"`
}

// A GuildEmojisUpdate is the data for a guild emoji update event.
type GuildEmojisUpdate struct {
	GuildID string   `json:"guild_id"`
	Emojis  []*Emoji `json:"emojis"`
}

// A GuildMembersChunk is the data for a GuildMembersChunk event.
type GuildMembersChunk struct {
	GuildID string    `json:"guild_id"`
	Members []*Member `json:"members"`
}

// GuildIntegrationsUpdate is the data for a GuildIntegrationsUpdate event.
type GuildIntegrationsUpdate struct {
	GuildID string `json:"guild_id"`
}

// MessageAck is the data for a MessageAck event.
type MessageAck struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
}

// MessageCreate is the data for a MessageCreate event.
type MessageCreate struct {
	*Message
}

// MessageUpdate is the data for a MessageUpdate event.
type MessageUpdate struct {
	*Message
	// BeforeUpdate will be nil if the Message was not previously cached in the state cache.
	BeforeUpdate *Message `json:"-"`
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
	Trace []string `json:"_trace"`
}

// TypingStart is the data for a TypingStart event.
type TypingStart struct {
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id,omitempty"`
	Timestamp int    `json:"timestamp"`
}

// UserUpdate is the data for a UserUpdate event.
type UserUpdate struct {
	*User
}

// VoiceServerUpdate is the data for a VoiceServerUpdate event.
type VoiceServerUpdate struct {
	Token    string `json:"token"`
	GuildID  string `json:"guild_id"`
	Endpoint string `json:"endpoint"`
}

// VoiceStateUpdate is the data for a VoiceStateUpdate event.
type VoiceStateUpdate struct {
	*VoiceState
}

// MessageDeleteBulk is the data for a MessageDeleteBulk event
type MessageDeleteBulk struct {
	Messages  []string `json:"ids"`
	ChannelID string   `json:"channel_id"`
	GuildID   string   `json:"guild_id"`
}

// WebhooksUpdate is the data for a WebhooksUpdate event
type WebhooksUpdate struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
}
