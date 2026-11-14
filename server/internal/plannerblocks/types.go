package plannerblocks

import "time"

// ProgressState is the public, server-derived state of a planner block.
type ProgressState string

const (
	ProgressPlanned   ProgressState = "planned"
	ProgressVerifying ProgressState = "verifying"
	ProgressFailed    ProgressState = "failed"
	ProgressReview    ProgressState = "review"
	ProgressDone      ProgressState = "done"
	ProgressUnknown   ProgressState = "unknown"
)

// Owner contains only public account fields loaded from the local accounts table.
type Owner struct {
	Username    string `json:"username" doc:"Account username"`
	DisplayName string `json:"displayName" doc:"Account display name"`
	Color       string `json:"color" doc:"Verified account color"`
}

// Approvals is the current GitLab merge request approval state.
type Approvals struct {
	Approved      bool     `json:"approved"`
	ApprovalsLeft int      `json:"approvalsLeft"`
	ApprovedBy    []string `json:"approvedBy"`
}

// Progress describes the latest observable GitLab state. ErrorCode is a stable
// public code; upstream GitLab errors are never returned to public clients.
type Progress struct {
	State          ProgressState `json:"state" enum:"planned,verifying,failed,review,done,unknown"`
	PipelineStatus string        `json:"pipelineStatus,omitempty"`
	ErrorCode      string        `json:"errorCode,omitempty"`
	Approvals      *Approvals    `json:"approvals,omitempty"`
}

// PlannerBlock is the full team planner representation. It is returned only to
// an authenticated team member; the public summary never includes these fields.
type PlannerBlock struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Position    int32      `json:"position"`
	Owner       Owner      `json:"owner"`
	Branch      string     `json:"branch"`
	MRIID       *int64     `json:"mrIid"`
	MRURL       string     `json:"mrUrl"`
	ParentID    *int64     `json:"parentId"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	Progress    Progress   `json:"progress"`
}

// PublicProgress is the only progress information exposed without a team
// session: the stage of the work, without pipeline, approval or MR details.
type PublicProgress struct {
	State ProgressState `json:"state" enum:"planned,verifying,failed,review,done,unknown"`
}

// PublicPlannerBlock is the compact plan shown to anonymous visitors. It
// deliberately contains no description, owner, branch or merge request data.
type PublicPlannerBlock struct {
	ID        int64          `json:"id"`
	Title     string         `json:"title"`
	Position  int32          `json:"position"`
	ParentID  *int64         `json:"parentId"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Progress  PublicProgress `json:"progress"`
}

// CreateInput is shared by the admin HTTP endpoint and CreateForAI. Owner
// identity and color are deliberately not accepted from clients.
type CreateInput struct {
	Title       string `json:"title" minLength:"1" maxLength:"200"`
	Description string `json:"description,omitempty" maxLength:"20000"`
	OwnerID     int64  `json:"ownerId" minimum:"1"`
	Branch      string `json:"branch,omitempty" maxLength:"255"`
	MRIID       *int64 `json:"mrIid,omitempty" minimum:"1"`
	ParentID    *int64 `json:"parentId,omitempty" minimum:"0"`
	Position    *int32 `json:"position,omitempty" minimum:"0"`
}

type createPlannerBlockInput struct {
	Body CreateInput
}

type updatePlannerBlockInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body CreateInput
}

type deletePlannerBlockInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type startPlannerBlockInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type plannerOutput struct {
	Body []PlannerBlock
}

type publicPlannerOutput struct {
	Body []PublicPlannerBlock
}

type plannerBlockOutput struct {
	Body PlannerBlock
}
