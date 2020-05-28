package main

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
	"github.com/vmihailenco/msgpack"
)

// A MarshalGuild holds all data related to a specific Discord Guild that is stored
// in cache.
type MarshalGuild struct {
	// The ID of the guild.
	ID string `msgpack:"id" json:"id"`

	// The name of the guild. (2–100 characters)
	Name string `msgpack:"name" json:"name"`

	// The hash of the guild's icon. Use Session.GuildIcon
	// to retrieve the icon itself.
	Icon string `msgpack:"icon" json:"icon"`

	// The voice region of the guild.
	Region string `msgpack:"region" json:"region"`

	// The ID of the AFK voice channel.
	AfkChannelID string `msgpack:"afk_channel_id" json:"afk_channel_id"`

	// The ID of the embed channel ID, used for embed widgets.
	EmbedChannelID string `msgpack:"embed_channel_id" json:"embed_channel_id"`

	// The user ID of the owner of the guild.
	OwnerID string `msgpack:"owner_id" json:"owner_id"`

	// The time at which the current user joined the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	JoinedAt Timestamp `msgpack:"joined_at" json:"joined_at"`

	// The hash of the guild's splash.
	Splash string `msgpack:"splash" json:"splash"`

	// The timeout, in seconds, before a user is considered AFK in voice.
	AfkTimeout int `msgpack:"afk_timeout" json:"afk_timeout"`

	// The number of members in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	MemberCount int `msgpack:"member_count" json:"member_count"`

	// The verification level required for the guild.
	VerificationLevel VerificationLevel `msgpack:"verification_level" json:"verification_level"`

	// Whether the guild has embedding enabled.
	EmbedEnabled bool `msgpack:"embed_enabled" json:"embed_enabled"`

	// Whether the guild is considered large. This is
	// determined by a member threshold in the identify packet,
	// and is currently hard-coded at 250 members in the library.
	Large bool `msgpack:"large" json:"large"`

	// The default message notification setting for the guild.
	// 0 == all messages, 1 == mentions only.
	DefaultMessageNotifications int `msgpack:"default_message_notifications" json:"default_message_notifications"`

	// A list of roles in the guild.
	Roles      []string `msgpack:"roles"`
	RoleValues []*Role  `json:"roles" msgpack:"-"`

	// A list of the custom emojis present in the guild.
	Emojis      []string `msgpack:"emojis"`
	EmojiValues []*Emoji `json:"emojis" msgpack:"-"`

	// A list of channels in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Channels      []string   `msgpack:"channels"`
	ChannelValues []*Channel `json:"channels" msgpack:"-"`

	// A list of the members in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Members []*Member `json:"members" msgpack:"-"`

	// Whether this guild is currently unavailable (most likely due to outage).
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Unavailable bool `msgpack:"unavailable" json:"unavailable"`

	// The explicit content filter level
	ExplicitContentFilter ExplicitContentFilterLevel `msgpack:"explicit_content_filter" json:"explicit_content_filter"`

	// The list of enabled guild features
	Features []string `msgpack:"features" json:"features"`

	// Required MFA level for the guild
	MfaLevel MfaLevel `msgpack:"mfa_level" json:"mfa_level"`

	// Whether or not the Server Widget is enabled
	WidgetEnabled bool `msgpack:"widget_enabled" json:"widget_enabled"`

	// The Channel ID for the Server Widget
	WidgetChannelID string `msgpack:"widget_channel_id" json:"widget_channel_id"`

	// The Channel ID to which system messages are sent (eg join and leave messages)
	SystemChannelID string `msgpack:"system_channel_id" json:"system_channel_id"`

	// the vanity url code for the guild
	VanityURLCode string `msgpack:"vanity_url_code" json:"vanity_url_code"`

	// the description for the guild
	Description string `msgpack:"description" json:"description"`

	// The hash of the guild's banner
	Banner string `msgpack:"banner" json:"banner"`

	// The premium tier of the guild
	PremiumTier PremiumTier `msgpack:"premium_tier" json:"premium_tier"`

	// The total number of users currently boosting this server
	PremiumSubscriptionCount int `msgpack:"premium_subscription_count" json:"premium_subscription_count"`
}

// Create takes a discord guild object in the form of a RawMessage
// and unmarshals the values and fills out the MarshalGuild struct.
func (mg *MarshalGuild) Create(data jsoniter.RawMessage) (err error) {

	err = json.Unmarshal(data, &mg)
	if err != nil {
		zlog.Error().Err(err).Msg("failed to unmarshal guild")
		return
	}

	mg.Roles = make([]string, 0)
	for _, r := range mg.RoleValues {
		mg.Roles = append(mg.Roles, r.ID)
	}

	mg.Channels = make([]string, 0)
	for _, c := range mg.ChannelValues {
		mg.Channels = append(mg.Channels, c.ID)
	}

	mg.Emojis = make([]string, 0)
	for _, e := range mg.EmojiValues {
		mg.Emojis = append(mg.Emojis, e.ID)
	}

	return
}

// From is similar to the Create method however is intended for Redis
// values which use msgpack instead of json and will not assign IDs from
// values which we want to preserve and are not stored in redis
func (mg *MarshalGuild) From(data []byte) (err error) {

	err = msgpack.Unmarshal(data, &mg)
	return
}

// Delete removes the guild object from redis
func (mg *MarshalGuild) Delete(m *Manager) (err error) {
	if len(mg.Roles) > 0 {
		if err = m.Configuration.redisClient.HDel(
			ctx,
			fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, mg.ID),
			mg.Roles...,
		).Err(); err != nil {
			return
		}
	}

	if len(mg.Channels) > 0 {
		if err = m.Configuration.redisClient.HDel(
			ctx,
			fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
			mg.Channels...,
		).Err(); err != nil {
			return
		}
	}

	if len(mg.Emojis) > 0 {
		if err = m.Configuration.redisClient.HDel(
			ctx,
			fmt.Sprintf("%s:emojis", m.Configuration.RedisPrefix),
			mg.Emojis...,
		).Err(); err != nil {
			return
		}
	}

	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
		mg.ID,
	).Err()

	return
}

// Save will store the guild, role, channels and emojis on redis
func (mg *MarshalGuild) Save(m *Manager) (err error) {
	var ma interface{}

	// We create a map of keys and values that redis will use instead
	// of creating multiple HSet requests.

	guildRoles := make(map[string]interface{})
	for _, r := range mg.RoleValues {
		if ma, err = msgpack.Marshal(r); err == nil {
			guildRoles[r.ID] = ma
		}
	}

	if len(guildRoles) > 0 {
		err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, mg.ID),
			guildRoles,
		).Err()
	}

	guildChannels := make(map[string]interface{})
	for _, c := range mg.ChannelValues {
		if ma, err = msgpack.Marshal(c); err == nil {
			guildChannels[c.ID] = ma
		}
	}

	if len(guildChannels) > 0 {
		err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
			guildChannels,
		).Err()
	}

	guildEmojis := make(map[string]interface{})
	for _, e := range mg.EmojiValues {
		if ma, err = msgpack.Marshal(e); err == nil {
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

	if ma, err = msgpack.Marshal(mg); err == nil {
		err = m.Configuration.redisClient.HSet(
			ctx,
			fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
			mg.ID,
			ma,
		).Err()
	}

	return
}
