package localaccounts

import "time"

const (
	// SessionCookieName is shared with the authentication middleware.
	SessionCookieName = "portal_session"
	sessionLifetime   = 8 * time.Hour
)

// Config contains the public origins and the single administrator identity.
type Config struct {
	PublicURL     string
	PlannerURL    string
	AdminUser     string
	AdminIP       string
	SecureCookies bool
}

// LocalAccount is the safe API representation of an account. It deliberately
// has no password hash or session fields.
type LocalAccount struct {
	ID              int64     `json:"id"`
	Username        string    `json:"username"`
	DisplayName     string    `json:"displayName"`
	Role            string    `json:"role"`
	Enabled         bool      `json:"enabled"`
	Color           string    `json:"color"`
	PersonalContext string    `json:"personalContext"`
	Tone            string    `json:"tone"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type LoginRequest struct {
	Username string `json:"username" minLength:"3" maxLength:"32" pattern:"^[a-z][a-z0-9_-]{2,31}$"`
	Password string `json:"password" minLength:"1" maxLength:"128"`
	Audience string `json:"audience" enum:"coding,admin"`
}

type LoginResult struct {
	CSRF    string       `json:"csrf"`
	Account LocalAccount `json:"account"`
}

type CreateAccountRequest struct {
	Username        string `json:"username" minLength:"3" maxLength:"32" pattern:"^[a-z][a-z0-9_-]{2,31}$"`
	DisplayName     string `json:"displayName" minLength:"1" maxLength:"100"`
	Password        string `json:"password" minLength:"12" maxLength:"128"`
	Role            string `json:"role" enum:"member,admin"`
	Color           string `json:"color" pattern:"^#[0-9A-Fa-f]{6}$"`
	PersonalContext string `json:"personalContext" maxLength:"16000"`
	Tone            string `json:"tone" maxLength:"2000"`
}

type ProfileRequest struct {
	DisplayName     string `json:"displayName" minLength:"1" maxLength:"100"`
	Color           string `json:"color" pattern:"^#[0-9A-Fa-f]{6}$"`
	PersonalContext string `json:"personalContext" maxLength:"16000"`
	Tone            string `json:"tone" maxLength:"2000"`
}

type PasswordRequest struct {
	Password string `json:"password" minLength:"12" maxLength:"128"`
}

type Conversation struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Phase     string    `json:"phase"`
	CreatedAt time.Time `json:"createdAt"`
}

type AccountMessage struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversationId"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}

type ConversationDetail struct {
	Conversation
	Messages []AccountMessage `json:"messages"`
}

type localLoginInput struct {
	Origin string `header:"Origin" maxLength:"2048"`
	Body   LoginRequest
}

type localLoginOutput struct {
	CacheControl string `header:"Cache-Control"`
	SetCookie    string `header:"Set-Cookie"`
	Body         LoginResult
}

type listAccountsInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"100"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}

type createAccountInput struct {
	Body CreateAccountRequest
}

type accountIDInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type updateProfileInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body ProfileRequest
}

type setEnabledInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

type setPasswordInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body PasswordRequest
}

type accountConversationsInput struct {
	ID     int64 `path:"id" minimum:"1"`
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"100"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}

type accountsOutput struct {
	Body []LocalAccount
}

type accountOutput struct {
	Body LocalAccount
}

type conversationsOutput struct {
	Body []Conversation
}

type conversationOutput struct {
	Body ConversationDetail
}
