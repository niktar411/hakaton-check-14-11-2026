package notifications

import "time"

// Notification is one event shown in the recipient's chat.
type Notification struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	CreatedAt      time.Time  `json:"createdAt"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
}

// Delivery limits for the chat notification list.
const (
	listLimit int32 = 50
)

// Event kinds stored in chat_notifications.kind.
const (
	KindBlockCompleted     = "block_completed"
	KindMergeRequestCreated = "merge_request_created"
)
