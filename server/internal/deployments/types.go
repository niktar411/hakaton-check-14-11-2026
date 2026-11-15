package deployments

import (
	"encoding/json"
	"time"
)

const (
	ActionDeploy             = "deploy"
	ActionRollback           = "rollback"
	ActionRestart            = "restart"
	ActionStart              = "start"
	ActionStop               = "stop"
	ActionCreateTestDatabase = "create_test_database"
	ActionDropTestDatabase   = "drop_test_database"

	TargetProduction = "production"
	TargetCoding     = "coding"
	TargetReview     = "review"
	TargetTest       = "test"
)

// Config contains the administrative boundary and credentials for the agent and
// GitLab webhook. Secrets are never included in API responses or audit details.
type Config struct {
	AdminUser           string
	AdminIP             string
	AgentURL            string
	AgentToken          string
	GitLabWebhookSecret string
}

// Deployment is the audit-safe representation of a deployment operation.
type Deployment struct {
	ID         int64           `json:"id"`
	Actor      string          `json:"actor"`
	Action     string          `json:"action"`
	Target     string          `json:"target"`
	Release    string          `json:"release"`
	Status     string          `json:"status"`
	BackupName string          `json:"backupName"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"createdAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
}

// MergeResult is read back from GitLab after the merge call. Merged is derived
// from the actual state rather than assumed from a successful HTTP response.
type MergeResult struct {
	IID    int64  `json:"iid"`
	State  string `json:"state"`
	SHA    string `json:"sha"`
	WebURL string `json:"webUrl"`
	Merged bool   `json:"merged"`
}

// AgentResult is the terminal result reported by the deployment agent. Details
// is input only; the service persists its own fixed, audit-safe details.
type AgentResult struct {
	Status     string          `json:"status"`
	BackupName string          `json:"backup_name"`
	Details    json.RawMessage `json:"details"`
}

type deploymentListInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"50"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}

type deploymentListOutput struct {
	Body []Deployment
}

type mergeRequestInput struct {
	IID  int64 `path:"iid" minimum:"1"`
	Body struct {
		SHA string `json:"sha" minLength:"1" maxLength:"128"`
	}
}

type mergeRequestOutput struct {
	Body MergeResult
}

type runDeploymentInput struct {
	Action string `path:"action" enum:"deploy,rollback,restart,start,stop,create_test_database,drop_test_database"`
	Body   struct {
		Target  string `json:"target" enum:"production,coding,review,test"`
		Release string `json:"release,omitempty" maxLength:"128"`
	}
}

type runDeploymentOutput struct {
	Body Deployment
}
