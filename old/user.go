package main

// A User stores all data for an individual Discord user.
type User struct {
	// The ID of the user.
	ID string `json:"id"`

	// The user's username.
	Username string `json:"username"`

	// The hash of the user's avatar. Use Session.UserAvatar
	// to retrieve the avatar itself.
	Avatar string `json:"avatar"`

	// The discriminator of the user (4 numbers after name).
	Discriminator string `json:"discriminator"`

	// The token of the user. This is only present for
	// the user represented by the current session.
	Token string `json:"token,omitempty" msgpack:"token,omitempty"`

	// Whether the user has multi-factor authentication enabled.
	MFAEnabled bool `json:"mfa_enabled"`

	// Whether the user is a bot.
	Bot bool `json:"bot"`
}
