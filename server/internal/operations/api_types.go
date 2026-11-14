package operations

import "time"

type Identity struct {
	Username   string `json:"username"`
	Admin      bool   `json:"admin"`
	CSRF       string `json:"csrf"`
	ModelReady bool   `json:"modelReady"`
}
type ContainerMetrics struct {
	Server      string  `json:"server"`
	Name        string  `json:"name"`
	Image       string  `json:"image"`
	State       string  `json:"state"`
	Health      string  `json:"health"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryUsed  int64   `json:"memoryUsed"`
	MemoryLimit int64   `json:"memoryLimit"`
	Restarts    int64   `json:"restarts"`
}
type Metrics struct {
	Name          string             `json:"name"`
	Available     bool               `json:"available"`
	Error         string             `json:"error,omitempty"`
	CPUPercent    float64            `json:"cpuPercent"`
	MemoryUsed    int64              `json:"memoryUsed"`
	MemoryTotal   int64              `json:"memoryTotal"`
	DiskUsed      int64              `json:"diskUsed"`
	DiskTotal     int64              `json:"diskTotal"`
	UptimeSeconds float64            `json:"uptimeSeconds"`
	SampledAt     time.Time          `json:"sampledAt"`
	Containers    []ContainerMetrics `json:"containers,omitempty"`
}
type Peer struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	Enabled         bool   `json:"enabled"`
	Protected       bool   `json:"protected"`
	LatestHandshake int64  `json:"latestHandshake"`
	ReceivedBytes   int64  `json:"receivedBytes"`
	SentBytes       int64  `json:"sentBytes"`
}
type UsageEvent struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	InputTokens  *int64    `json:"inputTokens"`
	OutputTokens *int64    `json:"outputTokens"`
	TotalTokens  *int64    `json:"totalTokens"`
	CreatedAt    time.Time `json:"createdAt"`
}
type UsageTotal struct {
	Username        string `json:"username"`
	Requests        int64  `json:"requests"`
	UnknownRequests int64  `json:"unknownRequests"`
	InputTokens     int64  `json:"inputTokens"`
	OutputTokens    int64  `json:"outputTokens"`
	TotalTokens     int64  `json:"totalTokens"`
}
type AuditEvent struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}
type Dashboard struct {
	Servers    []Metrics          `json:"servers"`
	Containers []ContainerMetrics `json:"containers"`
	Peers      []Peer             `json:"peers"`
	PeerError  string             `json:"peerError,omitempty"`
	Totals     []UsageTotal       `json:"totals"`
	Audit      []AuditEvent       `json:"audit"`
}
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type ChatResult struct {
	ID       int64     `json:"id"`
	Phase    string    `json:"phase"`
	Messages []Message `json:"messages"`
}
type ChatSummary struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Phase        string    `json:"phase"`
	MessageCount int       `json:"messageCount"`
	CreatedAt    time.Time `json:"createdAt"`
}
type ChatHistory struct {
	ID       int64     `json:"id"`
	Phase    string    `json:"phase"`
	Messages []Message `json:"messages"`
}

// ChatUpdate describes one completed planner block delivered to a member as a
// chat prompt. Every member receives exactly one update per block.
type ChatUpdate struct {
	ID          int64     `json:"id"`
	BlockID     int64     `json:"blockId"`
	Title       string    `json:"title"`
	Branch      string    `json:"branch"`
	MRURL       string    `json:"mrUrl"`
	OwnerName   string    `json:"ownerName"`
	Description string    `json:"description"`
	CompletedAt time.Time `json:"completedAt"`
}
type meOutput struct{ Body Identity }
type dashboardOutput struct{ Body Dashboard }
type usageInput struct {
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}
type usageOutput struct{ Body []UsageEvent }
type peerInput struct {
	ID   string `path:"id" enum:"egor,leonid,igor"`
	Body struct {
		Action string `json:"action" enum:"enable,disable,rotate"`
	}
}
type PeerResult struct {
	Configuration string `json:"configuration,omitempty"`
	ID            string `json:"id"`
}
type peerOutput struct{ Body PeerResult }
type chatInput struct {
	Body struct {
		ID        int64  `json:"id" minimum:"0"`
		Prompt    string `json:"prompt" minLength:"1" maxLength:"16000"`
		Confirm   bool   `json:"confirm"`
		RequestID string `json:"requestId" minLength:"16" maxLength:"80" pattern:"^[a-zA-Z0-9-]+$"`
	}
}
type chatOutput struct{ Body ChatResult }
type chatConversationsInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"50"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}
type chatConversationsOutput struct{ Body []ChatSummary }
type chatHistoryInput struct {
	ID int64 `path:"id" minimum:"1"`
}
type chatHistoryOutput struct{ Body ChatHistory }
type chatUpdatesOutput struct{ Body []ChatUpdate }
type chatUpdatePullInput struct {
	ID int64 `path:"id" minimum:"1"`
}
type chatUpdatePullOutput struct{ Body ChatUpdate }

// ChatNotification is one team event shown in the chat thread.
type ChatNotification struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	CreatedAt      time.Time  `json:"createdAt"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
}
type chatNotificationsInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"50" default:"20"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}
type chatNotificationsOutput struct{ Body []ChatNotification }
type chatNotificationAckInput struct {
	ID int64 `path:"id" minimum:"1"`
}
type chatNotificationAckOutput struct{ Body struct{} }
