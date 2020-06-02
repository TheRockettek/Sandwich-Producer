package main

import (
	"fmt"
	"time"

	"github.com/vmihailenco/msgpack"
)

// Valid GameType values
const (
	GameTypeGame GameType = iota
	GameTypeStreaming
	GameTypeListening
	GameTypeWatching
)

// Constants for Status with the different current available status
const (
	StatusOnline       Status = "online"
	StatusIdle         Status = "idle"
	StatusDoNotDisturb Status = "dnd"
	StatusInvisible    Status = "invisible"
	StatusOffline      Status = "offline"
)

// Block contains known ChannelType values
const (
	ChannelTypeGuildText ChannelType = iota
	ChannelTypeDM
	ChannelTypeGuildVoice
	ChannelTypeGroupDM
	ChannelTypeGuildCategory
	ChannelTypeGuildNews
	ChannelTypeGuildStore
)

// Block contains the valid known MessageType values
const (
	MessageTypeDefault MessageType = iota
	MessageTypeRecipientAdd
	MessageTypeRecipientRemove
	MessageTypeCall
	MessageTypeChannelNameChange
	MessageTypeChannelIconChange
	MessageTypeChannelPinnedMessage
	MessageTypeGuildMemberJoin
	MessageTypeUserPremiumGuildSubscription
	MessageTypeUserPremiumGuildSubscriptionTierOne
	MessageTypeUserPremiumGuildSubscriptionTierTwo
	MessageTypeUserPremiumGuildSubscriptionTierThree
	MessageTypeChannelFollowAdd
)

// Constants for the different types of Message Activity
const (
	MessageActivityTypeJoin = iota + 1
	MessageActivityTypeSpectate
	MessageActivityTypeListen
	MessageActivityTypeJoinRequest
)

// Constants for the different bit offsets of Message Flags
const (
	// This message has been published to subscribed channels (via Channel Following)
	MessageFlagCrossposted = 1 << iota
	// This message originated from a message in another channel (via Channel Following)
	MessageFlagIsCrosspost
	// Do not include any embeds when serializing this message
	MessageFlagSuppressEmbeds
)

// Constants for VerificationLevel levels from 0 to 4 inclusive
const (
	VerificationLevelNone VerificationLevel = iota
	VerificationLevelLow
	VerificationLevelMedium
	VerificationLevelHigh
	VerificationLevelVeryHigh
)

// Constants for ExplicitContentFilterLevel levels from 0 to 2 inclusive
const (
	ExplicitContentFilterDisabled ExplicitContentFilterLevel = iota
	ExplicitContentFilterMembersWithoutRoles
	ExplicitContentFilterAllMembers
)

// Constants for MfaLevel levels from 0 to 1 inclusive
const (
	MfaLevelNone MfaLevel = iota
	MfaLevelElevated
)

// Constants for PremiumTier levels from 0 to 3 inclusive
const (
	PremiumTierNone PremiumTier = iota
	PremiumTier1
	PremiumTier2
	PremiumTier3
)

// GameType type definition
type GameType int

// Status type definition
type Status string

// Timestamp stores a timestamp, as sent by the Discord API.
type Timestamp string

// ChannelType is the type of a Channel
type ChannelType int

// MessageType is the type of Message
type MessageType int

// MessageActivityType is the type of message activity
type MessageActivityType int

// MessageFlag describes an extra feature of the message
type MessageFlag int

// VerificationLevel type definition
type VerificationLevel int

// ExplicitContentFilterLevel type definition
type ExplicitContentFilterLevel int

// MfaLevel type definition
type MfaLevel int

// PremiumTier type definition
type PremiumTier int

// ShardDisconnectOp is the payload for SESSION_DISCONNECT
type ShardDisconnectOp struct {
	ShardID    int `msgpack:"shard_id"`
	StatusCode int `msgpack:"status"`
}

// RequestGuildMembersData contains the data that is sent during a
// REQUEST_GUILD_MEMBERS payload.
type RequestGuildMembersData struct {
	GuildID string `json:"guild_id"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

// RequestGuildMembersOp contains the structure of the payload
type RequestGuildMembersOp struct {
	Op   int                     `json:"op"`
	Data RequestGuildMembersData `json:"d"`
}

// A TooManyRequests struct holds information received from Discord
// when receiving a HTTP 429 response.
type TooManyRequests struct {
	Bucket     string        `json:"bucket" msgpack:"bucket"`
	Message    string        `json:"message" msgpack:"message"`
	RetryAfter time.Duration `json:"retry_after" msgpack:"retry_after"`
}

// UpdateStatusData represents the status changed
type UpdateStatusData struct {
	IdleSince *int   `json:"since" msgpack:"since"`
	Game      *Game  `json:"game" msgpack:"game"`
	AFK       bool   `json:"afk" msgpack:"afk"`
	Status    string `json:"status" msgpack:"status"`
}

// A Game struct holds the name of the "playing .." game for a user
type Game struct {
	Name          string     `json:"name" msgpack:"name"`
	Type          GameType   `json:"type" msgpack:"type"`
	URL           string     `json:"url,omitempty" msgpack:"url,omitempty"`
	Details       string     `json:"details,omitempty" msgpack:"details,omitempty"`
	State         string     `json:"state,omitempty" msgpack:"state,omitempty"`
	TimeStamps    TimeStamps `json:"timestamps,omitempty" msgpack:"timestamps,omitempty"`
	Assets        Assets     `json:"assets,omitempty" msgpack:"assets,omitempty"`
	ApplicationID string     `json:"application_id,omitempty" msgpack:"application_id,omitempty"`
	Instance      int8       `json:"instance,omitempty" msgpack:"instance,omitempty"`
}

// A TimeStamps struct contains start and end times used in the rich presence "playing .." Game
type TimeStamps struct {
	EndTimestamp   int64 `json:"end,omitempty" msgpack:"end,omitempty"`
	StartTimestamp int64 `json:"start,omitempty" msgpack:"start,omitempty"`
}

// An Assets struct contains assets and labels used in the rich presence "playing .." Game
type Assets struct {
	LargeImageID string `json:"large_image,omitempty" msgpack:"large_image,omitempty"`
	SmallImageID string `json:"small_image,omitempty" msgpack:"small_image,omitempty"`
	LargeText    string `json:"large_text,omitempty" msgpack:"large_text,omitempty"`
	SmallText    string `json:"small_text,omitempty" msgpack:"small_text,omitempty"`
}

// A VoiceState stores the voice states of Guilds
type VoiceState struct {
	UserID    string `json:"user_id" msgpack:"user_id"`
	ChannelID string `json:"channel_id" msgpack:"channel_id"`
	GuildID   string `json:"guild_id" msgpack:"guild_id"`
	Suppress  bool   `json:"suppress" msgpack:"suppress"`
	SelfMute  bool   `json:"self_mute" msgpack:"self_mute"`
	SelfDeaf  bool   `json:"self_deaf" msgpack:"self_deaf"`
	Mute      bool   `json:"mute" msgpack:"mute"`
	Deaf      bool   `json:"deaf" msgpack:"deaf"`
}

// A User stores all data for an individual Discord user.
type User struct {
	// The ID of the user.
	ID string `json:"id" msgpack:"id"`

	// The user's username.
	Username string `json:"username" msgpack:"username"`

	// The hash of the user's avatar. Use Session.UserAvatar
	// to retrieve the avatar itself.
	Avatar string `json:"avatar" msgpack:"avatar"`

	// The discriminator of the user (4 numbers after name).
	Discriminator string `json:"discriminator" msgpack:"discriminator"`

	// The token of the user. This is only present for
	// the user represented by the current session.
	Token string `json:"token,omitempty" msgpack:"token,omitempty" msgpack:"token,omitempty"`

	// Whether the user has multi-factor authentication enabled.
	MFAEnabled bool `json:"mfa_enabled" msgpack:"mfa_enabled"`

	// Whether the user is a bot.
	Bot bool `json:"bot" msgpack:"bot"`

	// List of guild ids of mutual guilds
	Mutual MutualGuilds `json:"-" msgpack:"-"`
}

// MutualGuilds stores information about mutual guilds for a user
type MutualGuilds struct {
	Guilds  LockSet
	Removed LockSet
}

// AddMutual adds a mutual guild
func (u *User) AddMutual(val string) (change bool, err error) {
	_, change = u.Mutual.Guilds.Add(val)
	u.Mutual.Removed.Remove(val)
	return
}

// RemoveMutual removes a mutual guild
func (u *User) RemoveMutual(val string) (change bool, err error) {
	_, change = u.Mutual.Guilds.Remove(val)
	u.Mutual.Removed.Add(val)
	return
}

// SaveMutual saves the User into redis
func (u *User) SaveMutual(m *Manager) (err error) {
	var vals []string

	vals = u.Mutual.Guilds.Get()
	if len(vals) > 0 {
		err = m.Configuration.redisClient.SAdd(
			ctx,
			fmt.Sprintf("%s:user:%s:mutual", m.Configuration.RedisPrefix, u.ID),
			vals,
		).Err()
	}

	vals = u.Mutual.Removed.Get()
	if len(vals) > 0 {
		err = m.Configuration.redisClient.SRem(
			ctx,
			fmt.Sprintf("%s:user:%s:mutual", m.Configuration.RedisPrefix, u.ID),
			vals,
		).Err()
	}

	u.Mutual.Removed.Values = make([]string, 0)

	return
}

// FetchMutual fills the mutual guilds set
func (u *User) FetchMutual(m *Manager) (err error) {
	mutual, err := m.Configuration.redisClient.SMembers(
		ctx,
		fmt.Sprintf("%s:user:%s:mutual", m.Configuration.RedisPrefix, u.ID),
	).Result()

	if err != nil {
		for _, gid := range mutual {
			u.Mutual.Guilds.Add(gid)
		}
	}

	return
}

// A Member stores user information for Guild members. A guild
// member represents a certain user's presence in a guild.
type Member struct {
	// The guild ID on which the member exists.
	GuildID string `json:"guild_id" msgpack:"guild_id"`

	// The time at which the member joined the guild, in ISO8601.
	JoinedAt Timestamp `json:"joined_at" msgpack:"joined_at"`

	// The nickname of the member, if they hs.statusave one.
	Nick string `json:"nick" msgpack:"nick"`

	// Whether the member is deafened at a guild level.
	Deaf bool `json:"deaf" msgpack:"deaf"`

	// Whether the member is muted at a guild level.
	Mute bool `json:"mute" msgpack:"mute"`

	// The underlying user on which the member is based.
	User *User `json:"user" msgpack:"-"`

	// Referenced user id
	ID string `json:"-" msgpack:"id"`

	// A list of IDs of the roles which are possessed by the member.
	Roles []string `json:"roles" msgpack:"roles"`

	// When the user used their Nitro boost on the server
	PremiumSince Timestamp `json:"premium_since" msgpack:"premium_since"`
}

// From creates a member object from bytes that is expected to be from
// redis
func (me *Member) From(data []byte, m *Manager) (err error) {

	err = msgpack.Unmarshal(data, &me)
	if err != nil {
		return
	}

	// We add the user
	println("ID:", me.ID)

	err = me.FetchUser(false, m)
	if err != nil {
		m.log.Error().Err(err).Msgf("error fetching user %s", me.ID)
	}
	err = me.User.FetchMutual(m)
	if err != nil {
		m.log.Error().Err(err).Msgf("failed to fetch mutual for user %s", me.ID)
	}

	if err != nil {
		m.log.Error().Err(err).Msgf("error fetching mutual for user %s", me.ID)
	}

	me.User.AddMutual(me.GuildID)
	err = me.User.SaveMutual(m)

	return
}

// FetchUser will attempt to get the user object for the member.
// If force is true, it will get the user reguardless if the user
// in the member struct is already filled out.
func (me *Member) FetchUser(force bool, m *Manager) (err error) {

	if force || me.User == &(User{}) {
		if me.ID != "" {
			u, err := m.getUser(me.ID)
			if err != nil {
				me.User = &u
			}
		} else {
			println("no id set")
		}
	} else {
		println("no need to fetch user")
	}

	return
}

// Marshaled saves the marshaled data
func (me *Member) Marshaled(updateUser bool, m *Manager) (ma []byte, err error) {
	me.ID = me.User.ID

	if res, err := m.Configuration.redisClient.HExists(
		ctx,
		fmt.Sprintf("%s:user", m.Configuration.RedisPrefix),
		me.ID,
	).Result(); err == nil {
		if res {
			u, _ := m.getUser(me.ID)
			me.User = &u
			me.User.FetchMutual(m)
			if err != nil {
				m.log.Error().Err(err).Msgf("error fetching mutual for user %s", me.ID)
			}
		}
	}

	me.User.AddMutual(me.GuildID)
	if updateUser {
		if err = me.User.SaveMutual(m); err != nil {
			m.log.Error().Err(err).Msg("failed to update user from redis")
		}
	}

	ma, err = msgpack.Marshal(me)

	return
}

// Save saves the Member into redis
func (me *Member) Save(m *Manager) (err error) {
	ma, err := me.Marshaled(true, m)
	if err != nil {
		m.log.Error().Err(err).Msg("failed to marshal user")
	}

	err = m.Configuration.redisClient.HSet(
		ctx,
		fmt.Sprintf("%s:guild:%s:members", m.Configuration.RedisPrefix, me.GuildID),
		me.User.ID,
		ma,
	).Err()

	return
}

// Delete deletes the Member from redis
func (me *Member) Delete(m *Manager) (err error) {
	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:guild:%s:members", m.Configuration.RedisPrefix, me.GuildID),
		me.User.ID,
	).Err()

	u, err := m.getUser(me.User.ID)
	if err != nil {
		m.log.Error().Err(err).Msg("failed to retrieve user from redis")
	}

	u.RemoveMutual(me.GuildID)
	err = u.SaveMutual(m)

	return
}

// UnavailableGuild is sent when you receive a GUILD_DELETE event. This contains
// the guild ID and if it is no longer available.
type UnavailableGuild struct {
	// The ID of the guild.
	ID string `json:"id" msgpack:"id"`

	// Whether this guild is currently unavailable (most likely due to outage).
	Unavailable bool `json:"unavailable" msgpack:"unavailable"`
}

// Delete removes the guild object from redis
func (ug *UnavailableGuild) Delete(m *Manager) (err error) {
	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:guilds", m.Configuration.RedisPrefix),
		ug.ID,
	).Err()

	return
}

// A Guild holds all data related to a specific Discord Guild.  Guilds are also
// sometimes referred to as Servers in the Discord client.
type Guild struct {
	// The ID of the guild.
	ID string `json:"id" msgpack:"id"`

	// The name of the guild. (2–100 characters)
	Name string `json:"name" msgpack:"name"`

	// The hash of the guild's icon. Use Session.GuildIcon
	// to retrieve the icon itself.
	Icon string `json:"icon" msgpack:"icon"`

	// The voice region of the guild.
	Region string `json:"region" msgpack:"region"`

	// The ID of the AFK voice channel.
	AfkChannelID string `json:"afk_channel_id" msgpack:"afk_channel_id"`

	// The ID of the embed channel ID, used for embed widgets.
	EmbedChannelID string `json:"embed_channel_id" msgpack:"embed_channel_id"`

	// The user ID of the owner of the guild.
	OwnerID string `json:"owner_id" msgpack:"owner_id"`

	// The time at which the current user joined the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	JoinedAt Timestamp `json:"joined_at" msgpack:"joined_at"`

	// The hash of the guild's splash.
	Splash string `json:"splash" msgpack:"splash"`

	// The timeout, in seconds, before a user is considered AFK in voice.
	AfkTimeout int `json:"afk_timeout" msgpack:"afk_timeout"`

	// The number of members in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	MemberCount int `json:"member_count" msgpack:"member_count"`

	// The verification level required for the guild.
	VerificationLevel VerificationLevel `json:"verification_level" msgpack:"verification_level"`

	// Whether the guild has embedding enabled.
	EmbedEnabled bool `json:"embed_enabled" msgpack:"embed_enabled"`

	// Whether the guild is considered large. This is
	// determined by a member threshold in the identify packet,
	// and is currently hard-coded at 250 members in the library.
	Large bool `json:"large" msgpack:"large"`

	// The default message notification setting for the guild.
	// 0 == all messages, 1 == mentions only.
	DefaultMessageNotifications int `json:"default_message_notifications" msgpack:"default_message_notifications"`

	// A list of roles in the guild.
	Roles []*Role `json:"roles" msgpack:"roles"`

	// A list of the custom emojis present in the guild.
	Emojis []*Emoji `json:"emojis" msgpack:"emojis"`

	// A list of the members in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Members []*Member `json:"members" msgpack:"-"`

	// A list of channels in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Channels []*Channel `json:"channels" msgpack:"channels"`

	// Whether this guild is currently unavailable (most likely due to outage).
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Unavailable bool `json:"unavailable" msgpack:"unavailable"`

	// The explicit content filter level
	ExplicitContentFilter ExplicitContentFilterLevel `json:"explicit_content_filter" msgpack:"explicit_content_filter"`

	// The list of enabled guild features
	Features []string `json:"features" msgpack:"features"`

	// Required MFA level for the guild
	MfaLevel MfaLevel `json:"mfa_level" msgpack:"mfa_level"`

	// Whether or not the Server Widget is enabled
	WidgetEnabled bool `json:"widget_enabled" msgpack:"widget_enabled"`

	// The Channel ID for the Server Widget
	WidgetChannelID string `json:"widget_channel_id" msgpack:"widget_channel_id"`

	// The Channel ID to which system messages are sent (eg join and leave messages)
	SystemChannelID string `json:"system_channel_id" msgpack:"system_channel_id"`

	// the vanity url code for the guild
	VanityURLCode string `json:"vanity_url_code" msgpack:"vanity_url_code"`

	// the description for the guild
	Description string `json:"description" msgpack:"description"`

	// The hash of the guild's banner
	Banner string `json:"banner" msgpack:"banner"`

	// The premium tier of the guild
	PremiumTier PremiumTier `json:"premium_tier" msgpack:"premium_tier"`

	// The total number of users currently boosting this server
	PremiumSubscriptionCount int `json:"premium_subscription_count" msgpack:"premium_subscription_count"`
}

// A Channel holds all data related to an individual Discord channel.
type Channel struct {
	// The ID of the channel.
	ID string `json:"id" msgpack:"id"`

	// The ID of the guild to which the channel belongs, if it is in a guild.
	// Else, this ID is empty (e.g. DM channels).
	GuildID string `json:"guild_id" msgpack:"guild_id"`

	// The name of the channel.
	Name string `json:"name" msgpack:"name"`

	// The topic of the channel.
	Topic string `json:"topic" msgpack:"topic,omitempty"`

	// The type of the channel.
	Type ChannelType `json:"type" msgpack:"type"`

	// The ID of the last message sent in the channel. This is not
	// guaranteed to be an ID of a valid message.
	LastMessageID string `json:"last_message_id" msgpack:"last_message_id,omitempty"`

	// The timestamp of the last pinned message in the channel.
	// Empty if the channel has no pinned messages.
	LastPinTimestamp Timestamp `json:"last_pin_timestamp" msgpack:"last_pin_timestamp,omitempty"`

	// Whether the channel is marked as NSFW.
	NSFW bool `json:"nsfw" msgpack:"nsfw,omitempty"`

	// Icon of the group DM channel.
	Icon string `json:"icon" msgpack:"icon"`

	// The position of the channel, used for sorting in client.
	Position int `json:"position" msgpack:"position"`

	// The bitrate of the channel, if it is a voice channel.
	Bitrate int `json:"bitrate" msgpack:"bitrate,omitempty"`

	// The recipients of the channel. This is only populated in DM channels.
	Recipients []*User `json:"recipients" msgpack:"recipients,omitempty"`

	// A list of permission overwrites present for the channel.
	PermissionOverwrites []*PermissionOverwrite `json:"permission_overwrites" msgpack:"permission_overwrites,omitempty"`

	// The user limit of the voice channel.
	UserLimit int `json:"user_limit" msgpack:"user_limit,omitempty"`

	// The ID of the parent channel, if the channel is under a category
	ParentID string `json:"parent_id" msgpack:"parent_id,omitempty"`

	// Amount of seconds a user has to wait before sending another message (0-21600)
	// bots, as well as users with the permission manage_messages or manage_channel, are unaffected
	RateLimitPerUser int `json:"rate_limit_per_user" msgpack:"rate_limit_per_user,omitempty"`
}

// Save saves the Channel into redis
func (c *Channel) Save(m *Manager) (err error) {
	ma, err := msgpack.Marshal(c)
	if err != nil {
		return
	}

	err = m.Configuration.redisClient.HSet(
		ctx,
		fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
		c.ID,
		ma,
	).Err()

	return
}

// Delete deletes the Channel from redis
func (c *Channel) Delete(m *Manager) (err error) {
	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:channels", m.Configuration.RedisPrefix),
		c.ID,
	).Err()

	return
}

// A Role stores information about Discord guild member roles.
type Role struct {
	// The ID of the role.
	ID string `json:"id" msgpack:"id"`

	// The name of the role.
	Name string `json:"name" msgpack:"name"`

	// Whether this role is managed by an integration, and
	// thus cannot be manually added to, or taken from, members.
	Managed bool `json:"managed" msgpack:"managed"`

	// Whether this role is mentionable.
	Mentionable bool `json:"mentionable" msgpack:"mentionable"`

	// Whether this role is hoisted (shows up separately in member list).
	Hoist bool `json:"hoist" msgpack:"hoist"`

	// The hex color of this role.
	Color int `json:"color" msgpack:"color"`

	// The position of this role in the guild's role hierarchy.
	Position int `json:"position" msgpack:"position"`

	// The permissions of the role on the guild (doesn't include channel overrides).
	// This is a combination of bit masks; the presence of a certain permission can
	// be checked by performing a bitwise AND between this int and the permission.
	Permissions int `json:"permissions" msgpack:"permissions"`
}

// Save saves the GuildRole into redis
func (r *Role) Save(guildID string, m *Manager) (err error) {
	ma, err := msgpack.Marshal(r)
	if err != nil {
		return
	}

	if err = m.Configuration.redisClient.HSet(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guildID),
		r.ID,
		ma,
	).Err(); err != nil {
		return
	}

	return
}

// Delete removes the emoji from redis
func (r *Role) Delete(guildID string, m *Manager) (err error) {
	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, guildID),
		r.ID,
	).Err()

	return
}

// A GuildRole stores data for guild roles.
type GuildRole struct {
	Role    *Role  `json:"role" msgpack:"role"`
	GuildID string `json:"guild_id" msgpack:"guild_id"`
}

// Save saves the GuildRole into redis
func (gr *GuildRole) Save(m *Manager) (err error) {
	ma, err := msgpack.Marshal(gr.Role)
	if err != nil {
		return
	}

	err = m.Configuration.redisClient.HSet(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, gr.GuildID),
		gr.Role.ID,
		ma,
	).Err()

	return
}

// Delete removes the emoji from redis
func (gr *GuildRole) Delete(m *Manager) (err error) {
	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:guild:%s:roles", m.Configuration.RedisPrefix, gr.GuildID),
		gr.Role.ID,
	).Err()

	return
}

// Emoji struct holds data related to Emoji's
type Emoji struct {
	ID            string   `json:"id" msgpack:"id"`
	Name          string   `json:"name" msgpack:"name"`
	Roles         []string `json:"roles" msgpack:"roles"`
	Managed       bool     `json:"managed" msgpack:"managed"`
	RequireColons bool     `json:"require_colons" msgpack:"require_colons"`
	Animated      bool     `json:"animated" msgpack:"animated"`
	Available     bool     `json:"available" msgpack:"available"`
}

// Save saves the emoji into redis
func (e *Emoji) Save(m *Manager) (err error) {
	ma, err := msgpack.Marshal(e)
	if err != nil {
		return
	}

	if err = m.Configuration.redisClient.HSet(
		ctx,
		fmt.Sprintf("%s:emojis", m.Configuration.RedisPrefix),
		e.ID,
		ma,
	).Err(); err != nil {
		return
	}

	return
}

// Delete removes the emoji from redis
func (e *Emoji) Delete(m *Manager) (err error) {
	err = m.Configuration.redisClient.HDel(
		ctx,
		fmt.Sprintf("%s:emojis", m.Configuration.RedisPrefix),
		e.ID,
	).Err()
	return
}

// A Message stores all data related to a specific Discord message.
type Message struct {
	// The ID of the message.
	ID string `json:"id" msgpack:"id"`

	// The ID of the channel in which the message was sent.
	ChannelID string `json:"channel_id" msgpack:"channel_id"`

	// The ID of the guild in which the message was sent.
	GuildID string `json:"guild_id,omitempty" msgpack:"guild_id,omitempty"`

	// The content of the message.
	Content string `json:"content" msgpack:"content"`

	// The time at which the messsage was sent.
	// CAUTION: this field may be removed in a
	// future API version; it is safer to calculate
	// the creation time via the ID.
	Timestamp Timestamp `json:"timestamp" msgpack:"timestamp"`

	// The time at which the last edit of the message
	// occurred, if it has been edited.
	EditedTimestamp Timestamp `json:"edited_timestamp" msgpack:"edited_timestamp"`

	// The roles mentioned in the message.
	MentionRoles []string `json:"mention_roles" msgpack:"mention_roles"`

	// Whether the message is text-to-speech.
	Tts bool `json:"tts" msgpack:"tts"`

	// Whether the message mentions everyone.
	MentionEveryone bool `json:"mention_everyone" msgpack:"mention_everyone"`

	// The author of the message. This is not guaranteed to be a
	// valid user (webhook-sent messages do not possess a full author).
	Author *User `json:"author" msgpack:"author"`

	// A list of attachments present in the message.
	Attachments []*MessageAttachment `json:"attachments" msgpack:"attachments"`

	// A list of embeds present in the message. Multiple
	// embeds can currently only be sent by webhooks.
	Embeds []*MessageEmbed `json:"embeds" msgpack:"embeds"`

	// A list of users mentioned in the message.
	Mentions []*User `json:"mentions" msgpack:"mentions"`

	// A list of reactions to the message.
	Reactions []*MessageReactions `json:"reactions" msgpack:"reactions"`

	// Whether the message is pinned or not.
	Pinned bool `json:"pinned" msgpack:"pinned"`

	// The type of the message.
	Type MessageType `json:"type" msgpack:"type" msgpack:"type,omitempty"`

	// The webhook ID of the message, if it was generated by a webhook
	WebhookID string `json:"webhook_id" msgpack:"webhook_id" msgpack:"webhook_id,omitempty"`

	// Member properties for this message's author,
	// contains only partial information
	Member *Member `json:"member" msgpack:"member"`

	// Channels specifically mentioned in this message
	// Not all channel mentions in a message will appear in mention_channels.
	// Only textual channels that are visible to everyone in a lurkable guild will ever be included.
	// Only crossposted messages (via Channel Following) currently include mention_channels at all.
	// If no mentions in the message meet these requirements, this field will not be sent.
	MentionChannels []*Channel `json:"mention_channels" msgpack:"mention_channels" msgpack:"mention_channels,omitempty"`

	// Is sent with Rich Presence-related chat embeds
	Activity *MessageActivity `json:"activity" msgpack:"activity" msgpack:"activity,omitempty"`

	// Is sent with Rich Presence-related chat embeds
	Application *MessageApplication `json:"application" msgpack:"application" msgpack:"application,omitempty"`

	// MessageReference contains reference data sent with crossposted messages
	MessageReference *MessageReference `json:"message_reference" msgpack:"message_reference" msgpack:"message_reference,omitempty"`

	// The flags of the message, which describe extra features of a message.
	// This is a combination of bit masks; the presence of a certain permission can
	// be checked by performing a bitwise AND between this int and the flag.
	Flags int `json:"flags" msgpack:"flags" msgpack:"flags,omitempty"`
}

// A MessageAttachment stores data for message attachments.
type MessageAttachment struct {
	ID       string `json:"id" msgpack:"id"`
	URL      string `json:"url" msgpack:"url"`
	ProxyURL string `json:"proxy_url" msgpack:"proxy_url"`
	Filename string `json:"filename" msgpack:"filename"`
	Width    int    `json:"width" msgpack:"width"`
	Height   int    `json:"height" msgpack:"height"`
	Size     int    `json:"size" msgpack:"size"`
}

// MessageEmbedFooter is a part of a MessageEmbed struct.
type MessageEmbedFooter struct {
	Text         string `json:"text,omitempty" msgpack:"text,omitempty"`
	IconURL      string `json:"icon_url,omitempty" msgpack:"icon_url,omitempty"`
	ProxyIconURL string `json:"proxy_icon_url,omitempty" msgpack:"proxy_icon_url,omitempty"`
}

// MessageEmbedImage is a part of a MessageEmbed struct.
type MessageEmbedImage struct {
	URL      string `json:"url,omitempty" msgpack:"url,omitempty"`
	ProxyURL string `json:"proxy_url,omitempty" msgpack:"proxy_url,omitempty"`
	Width    int    `json:"width,omitempty" msgpack:"width,omitempty"`
	Height   int    `json:"height,omitempty" msgpack:"height,omitempty"`
}

// MessageEmbedThumbnail is a part of a MessageEmbed struct.
type MessageEmbedThumbnail struct {
	URL      string `json:"url,omitempty" msgpack:"url,omitempty"`
	ProxyURL string `json:"proxy_url,omitempty" msgpack:"proxy_url,omitempty"`
	Width    int    `json:"width,omitempty" msgpack:"width,omitempty"`
	Height   int    `json:"height,omitempty" msgpack:"height,omitempty"`
}

// MessageEmbedVideo is a part of a MessageEmbed struct.
type MessageEmbedVideo struct {
	URL      string `json:"url,omitempty" msgpack:"url,omitempty"`
	ProxyURL string `json:"proxy_url,omitempty" msgpack:"proxy_url,omitempty"`
	Width    int    `json:"width,omitempty" msgpack:"width,omitempty"`
	Height   int    `json:"height,omitempty" msgpack:"height,omitempty"`
}

// MessageEmbedProvider is a part of a MessageEmbed struct.
type MessageEmbedProvider struct {
	URL  string `json:"url,omitempty" msgpack:"url,omitempty"`
	Name string `json:"name,omitempty" msgpack:"name,omitempty"`
}

// MessageEmbedAuthor is a part of a MessageEmbed struct.
type MessageEmbedAuthor struct {
	URL          string `json:"url,omitempty" msgpack:"url,omitempty"`
	Name         string `json:"name,omitempty" msgpack:"name,omitempty"`
	IconURL      string `json:"icon_url,omitempty" msgpack:"icon_url,omitempty"`
	ProxyIconURL string `json:"proxy_icon_url,omitempty" msgpack:"proxy_icon_url,omitempty"`
}

// MessageEmbedField is a part of a MessageEmbed struct.
type MessageEmbedField struct {
	Name   string `json:"name,omitempty" msgpack:"name,omitempty"`
	Value  string `json:"value,omitempty" msgpack:"value,omitempty"`
	Inline bool   `json:"inline,omitempty" msgpack:"inline,omitempty"`
}

// An MessageEmbed stores data for message embeds.
type MessageEmbed struct {
	URL         string                 `json:"url,omitempty" msgpack:"url,omitempty"`
	Type        string                 `json:"type,omitempty" msgpack:"type,omitempty"`
	Title       string                 `json:"title,omitempty" msgpack:"title,omitempty"`
	Description string                 `json:"description,omitempty" msgpack:"description,omitempty"`
	Timestamp   string                 `json:"timestamp,omitempty" msgpack:"timestamp,omitempty"`
	Color       int                    `json:"color,omitempty" msgpack:"color,omitempty"`
	Footer      *MessageEmbedFooter    `json:"footer,omitempty" msgpack:"footer,omitempty"`
	Image       *MessageEmbedImage     `json:"image,omitempty" msgpack:"image,omitempty"`
	Thumbnail   *MessageEmbedThumbnail `json:"thumbnail,omitempty" msgpack:"thumbnail,omitempty"`
	Video       *MessageEmbedVideo     `json:"video,omitempty" msgpack:"video,omitempty"`
	Provider    *MessageEmbedProvider  `json:"provider,omitempty" msgpack:"provider,omitempty"`
	Author      *MessageEmbedAuthor    `json:"author,omitempty" msgpack:"author,omitempty"`
	Fields      []*MessageEmbedField   `json:"fields,omitempty" msgpack:"fields,omitempty"`
}

// MessageReactions holds a reactions object for a message.
type MessageReactions struct {
	Count int    `json:"count" msgpack:"count"`
	Me    bool   `json:"me" msgpack:"me"`
	Emoji *Emoji `json:"emoji" msgpack:"emoji"`
}

// MessageActivity is sent with Rich Presence-related chat embeds
type MessageActivity struct {
	Type    MessageActivityType `json:"type" msgpack:"type"`
	PartyID string              `json:"party_id" msgpack:"party_id"`
}

// MessageApplication is sent with Rich Presence-related chat embeds
type MessageApplication struct {
	ID          string `json:"id" msgpack:"id"`
	CoverImage  string `json:"cover_image" msgpack:"cover_image"`
	Description string `json:"description" msgpack:"description"`
	Icon        string `json:"icon" msgpack:"icon"`
	Name        string `json:"name" msgpack:"name"`
}

// MessageReference contains reference data sent with crossposted messages
type MessageReference struct {
	MessageID string `json:"message_id" msgpack:"message_id"`
	ChannelID string `json:"channel_id" msgpack:"channel_id"`
	GuildID   string `json:"guild_id" msgpack:"guild_id"`
}

// A PermissionOverwrite holds permission overwrite data for a Channel
type PermissionOverwrite struct {
	ID    string `json:"id" msgpack:"id"`
	Type  string `json:"type" msgpack:"type"`
	Deny  int    `json:"deny" msgpack:"deny"`
	Allow int    `json:"allow" msgpack:"allow"`
}

// MessageReaction stores the data for a message reaction.
type MessageReaction struct {
	UserID    string `json:"user_id" msgpack:"user_id"`
	MessageID string `json:"message_id" msgpack:"message_id"`
	Emoji     Emoji  `json:"emoji" msgpack:"emoji"`
	ChannelID string `json:"channel_id" msgpack:"channel_id"`
	GuildID   string `json:"guild_id,omitempty" msgpack:"guild_id,omitempty"`
}

// GatewayBotResponse stores the data for the gateway/bot response
type GatewayBotResponse struct {
	URL          string        `json:"url" msgpack:"url"`
	Shards       int           `json:"shards" msgpack:"shards"`
	SessionLimit SessionLimits `json:"session_start_limit" msgpack:"session_start_limit"`
}

// SessionLimits stores data for the session_start_limit value of gateway response
type SessionLimits struct {
	Total          int           `json:"total" msgpack:"total"`
	Remaining      int           `json:"remaining" msgpack:"remaining"`
	ResetAfter     time.Duration `json:"reset_after" msgpack:"reset_after"`
	MaxConcurrency int           `json:"max_concurrency" msgpack:"max_concurrency"`
}
