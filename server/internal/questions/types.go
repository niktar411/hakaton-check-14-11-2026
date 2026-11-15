package questions

import "time"

const (
	maxQuestionLength  = 4000
	maxGroundingLength = 100000
	maxAnswerLength    = 8000
)

// Link is a canonical link selected from the server-owned registry.
type Link struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

// Answer keeps answer text separate from links so clients can render it as text.
type Answer struct {
	Answer string `json:"answer"`
	Links  []Link `json:"links"`
}

// SiteConfig describes the public applications and links the FAQ may reference.
type SiteConfig struct {
	PublicBaseURL  string `json:"publicBaseUrl"`
	PlannerBaseURL string `json:"plannerBaseUrl"`
	Links          []Link `json:"links"`
}

// Context is the administrator-controlled factual grounding for the model.
type Context struct {
	Context string `json:"context" maxLength:"100000"`
}

// HistoryEntry is one persisted public question and its grounded answer.
type HistoryEntry struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Links     []Link    `json:"links"`
	CreatedAt time.Time `json:"createdAt"`
}

// History is a limit/offset page. A total count is intentionally not fabricated.
type History struct {
	Items  []HistoryEntry `json:"items"`
	Limit  int32          `json:"limit"`
	Offset int32          `json:"offset"`
}

type publicQuestionInput struct {
	Body struct {
		Question  string `json:"question" minLength:"1" maxLength:"4000"`
		RequestID string `json:"requestId" minLength:"16" maxLength:"80" pattern:"^[a-zA-Z0-9-]+$"`
	}
}

type publicQuestionOutput struct {
	Body Answer
}

type siteConfigOutput struct {
	Body SiteConfig
}

type contextInput struct {
	Body Context
}

type contextOutput struct {
	Body Context
}

type historyInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"50"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}

type historyOutput struct {
	Body History
}
