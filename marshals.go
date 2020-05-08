package main

import (
	"encoding/json"
	"sync"

	"github.com/vmihailenco/msgpack"
)

// Marshallers stores all event marshallers to be sent through NATS
type Marshallers struct {
	sync.Mutex
	Marshalers map[string]func(Event) interface{}
}

// A MarshaledGuild stores the interface that is converted when stored
// in redis. This modifies the role and channel list
type MarshaledGuild struct {
	// The ID of the guild.
	ID string `json:"id"`

	// The name of the guild. (2–100 characters)
	Name string `json:"name"`

	// The hash of the guild's icon. Use Session.GuildIcon
	// to retrieve the icon itself.
	Icon string `json:"icon"`

	// The voice region of the guild.
	Region string `json:"region"`

	// The ID of the AFK voice channel.
	AfkChannelID string `json:"afk_channel_id"`

	// The ID of the embed channel ID, used for embed widgets.
	EmbedChannelID string `json:"embed_channel_id"`

	// The user ID of the owner of the guild.
	OwnerID string `json:"owner_id"`

	// The time at which the current user joined the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	JoinedAt Timestamp `json:"joined_at"`

	// The hash of the guild's splash.
	Splash string `json:"splash"`

	// The timeout, in seconds, before a user is considered AFK in voice.
	AfkTimeout int `json:"afk_timeout"`

	// The number of members in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	MemberCount int `json:"member_count"`

	// The verification level required for the guild.
	VerificationLevel VerificationLevel `json:"verification_level"`

	// Whether the guild has embedding enabled.
	EmbedEnabled bool `json:"embed_enabled"`

	// Whether the guild is considered large. This is
	// determined by a member threshold in the identify packet,
	// and is currently hard-coded at 250 members in the library.
	Large bool `json:"large"`

	// The default message notification setting for the guild.
	// 0 == all messages, 1 == mentions only.
	DefaultMessageNotifications int `json:"default_message_notifications"`

	// A list of roles in the guild.
	Roles []string `json:"roles"`

	// A list of the custom emojis present in the guild.
	Emojis []string `json:"emojis"`

	// A list of partial presence objects for members in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Presences []*Presence `json:"-"`

	// A list of channels in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Channels []string `json:"channels"`

	// Whether this guild is currently unavailable (most likely due to outage).
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	Unavailable bool `json:"unavailable"`

	// The explicit content filter level
	ExplicitContentFilter ExplicitContentFilterLevel `json:"explicit_content_filter"`

	// The list of enabled guild features
	Features []string `json:"features"`

	// Required MFA level for the guild
	MfaLevel MfaLevel `json:"mfa_level"`

	// Whether or not the Server Widget is enabled
	WidgetEnabled bool `json:"widget_enabled"`

	// The Channel ID for the Server Widget
	WidgetChannelID string `json:"widget_channel_id"`

	// The Channel ID to which system messages are sent (eg join and leave messages)
	SystemChannelID string `json:"system_channel_id"`

	// the vanity url code for the guild
	VanityURLCode string `json:"vanity_url_code"`

	// the description for the guild
	Description string `json:"description"`

	// The hash of the guild's banner
	Banner string `json:"banner"`

	// The premium tier of the guild
	PremiumTier PremiumTier `json:"premium_tier"`

	// The total number of users currently boosting this server
	PremiumSubscriptionCount int `json:"premium_subscription_count"`

	// A list of roles in the guild.
	RoleValues map[string]interface{} `json:"roles"`

	// A list of the custom emojis present in the guild.
	EmojiValues map[string]interface{} `json:"emojis"`

	// A list of channels in the guild.
	// This field is only present in GUILD_CREATE events and websocket
	// update events, and thus is only present in state-cached guilds.
	ChannelValues map[string]interface{} `json:"channels"`
}

// MarshalGuild creates a redis marshal worthy format of the guild object
func (s *Session) MarshalGuild(guild *Guild) (mg MarshaledGuild) {
	_guild := MarshaledGuild{
		ID:                          guild.ID,
		Name:                        guild.Name,
		Icon:                        guild.Icon,
		Region:                      guild.Region,
		AfkChannelID:                guild.AfkChannelID,
		EmbedChannelID:              guild.EmbedChannelID,
		OwnerID:                     guild.OwnerID,
		JoinedAt:                    guild.JoinedAt,
		Splash:                      guild.Splash,
		AfkTimeout:                  guild.AfkTimeout,
		MemberCount:                 guild.MemberCount,
		VerificationLevel:           guild.VerificationLevel,
		EmbedEnabled:                guild.EmbedEnabled,
		Large:                       guild.Large,
		DefaultMessageNotifications: guild.DefaultMessageNotifications,
		Roles:                       make([]string, 0),
		Emojis:                      make([]string, 0),
		Channels:                    make([]string, 0),
		RoleValues:                  make(map[string]interface{}),
		EmojiValues:                 make(map[string]interface{}),
		ChannelValues:               make(map[string]interface{}),
		Unavailable:                 guild.Unavailable,
		ExplicitContentFilter:       guild.ExplicitContentFilter,
		Features:                    guild.Features,
		MfaLevel:                    guild.MfaLevel,
		WidgetEnabled:               guild.WidgetEnabled,
		WidgetChannelID:             guild.WidgetChannelID,
		SystemChannelID:             guild.SystemChannelID,
		VanityURLCode:               guild.VanityURLCode,
		Description:                 guild.Description,
		Banner:                      guild.Banner,
		PremiumTier:                 guild.PremiumTier,
		PremiumSubscriptionCount:    guild.PremiumSubscriptionCount,
	}

	for _, c := range guild.Channels {
		_c, err := msgpack.Marshal(c)
		if err != nil {
			s.log.Error().Err(err).Str("id", c.ID).Msg("failed to marshal channel")
		}
		_guild.ChannelValues[c.ID] = _c
		_guild.Channels = append(_guild.Channels, c.ID)
	}

	for _, r := range guild.Roles {
		_r, err := msgpack.Marshal(r)
		if err != nil {
			s.log.Error().Err(err).Str("id", r.ID).Msg("failed to marshal role")
		}
		_guild.RoleValues[r.ID] = _r
		_guild.Roles = append(_guild.Roles, r.ID)
	}

	for _, e := range guild.Emojis {
		_e, err := msgpack.Marshal(e)
		if err != nil {
			s.log.Error().Err(err).Str("id", e.ID).Msg("failed to marshal emojis")
		}
		_guild.EmojiValues[e.ID] = _e
		_guild.Emojis = append(_guild.Emojis, e.ID)
	}

	return _guild
}

// addMarshaler adds a marshaller for a specific event. This is used to clean payloads
// before being sent over NATS.
func (m *Marshallers) addMarshaler(event string, marshaler func(Event) interface{}) {
	m.Lock()
	defer m.Unlock()
	m.Marshalers[event] = marshaler
}

var marshallers = Marshallers{
	Marshalers: make(map[string]func(Event) interface{}),
}

func init() {
	marshallers.addMarshaler("READY", func(e Event) interface{} {
		j, _ := json.Marshal(e.Struct)
		println(string(j))
		return e
	})
}
