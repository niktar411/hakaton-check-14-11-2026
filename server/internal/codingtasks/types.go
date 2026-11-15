package codingtasks

import "time"

const (
	PhaseClarifying           = "clarifying"
	PhaseAwaitingConfirmation = "awaiting_confirmation"
	PhaseRunning              = "running"
	PhaseAwaitingMerge        = "awaiting_merge"
	PhaseFailed               = "failed"
)

// CodingTask is the public representation of a coding task.
type CodingTask struct {
	ID              int64     `json:"id"`
	AccountID       int64     `json:"accountId"`
	ParentID        *int64    `json:"parentId,omitempty"`
	Request         string    `json:"request"`
	Phase           string    `json:"phase"`
	Clarification   string    `json:"clarification"`
	Branch          string    `json:"branch,omitempty"`
	MergeRequestIID *int64    `json:"mergeRequestIid,omitempty"`
	MergeRequestURL string    `json:"mergeRequestUrl,omitempty"`
	FailureReason   string    `json:"failureReason,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// CodingMessage is an immutable audit message. Tool messages contain a JSON
// result so arguments and results remain available without another table.
type CodingMessage struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ToolName  string    `json:"toolName,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// CodingTaskDetails includes the complete conversation for a task.
type CodingTaskDetails struct {
	CodingTask
	Messages []CodingMessage `json:"messages"`
}

type createCodingTaskInput struct {
	Body struct {
		Request  string `json:"request" minLength:"1" maxLength:"16000"`
		ParentID *int64 `json:"parentId,omitempty" minimum:"1"`
	}
}

type codingTaskIDInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type answerCodingTaskInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Answer string `json:"answer" minLength:"1" maxLength:"16000"`
	}
}

type confirmCodingTaskInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Confirmation string `json:"confirmation" maxLength:"200"`
		Understood   string `json:"understood" maxLength:"200"`
	}
}

type reworkCodingTaskInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Request string `json:"request" minLength:"1" maxLength:"16000"`
	}
}

type listCodingTasksInput struct {
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}

type codingTaskOutput struct {
	Body CodingTask
}

type codingTaskDetailsOutput struct {
	Body CodingTaskDetails
}

type codingTaskListOutput struct {
	Body []CodingTask
}
