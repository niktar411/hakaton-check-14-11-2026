package matching

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"hackathon/internal/identity"
)

type SamMatchingCandidate struct {
	ID             int64      `json:"id"`
	InstallationID int64      `json:"installationId"`
	ProductID      int64      `json:"productId"`
	RawName        string     `json:"rawName"`
	ProductName    string     `json:"productName"`
	DeviceName     string     `json:"deviceName"`
	Score          float32    `json:"score"`
	Method         string     `json:"method"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	ResolvedBy     string     `json:"resolvedBy,omitempty"`
	LastSeenAt     time.Time  `json:"lastSeenAt"`
}

type SamMatchingRunResult struct {
	Limit      int32 `json:"limit"`
	Scanned    int64 `json:"scanned"`
	Unmatched  int64 `json:"unmatched"`
	Matched    int64 `json:"matched"`
	Candidates int64 `json:"candidates"`
	Skipped    int64 `json:"skipped"`
	Pending    int64 `json:"pending"`
}

type SamMatchingSummary struct {
	Unmatched int64 `json:"unmatched"`
	Pending   int64 `json:"pending"`
	Confirmed int64 `json:"confirmed"`
	Rejected  int64 `json:"rejected"`
}

type samMatchingRunInput struct {
	Body struct {
		Limit int32 `json:"limit" minimum:"0" maximum:"5000"`
	}
}

type samMatchingRunOutput struct{ Body SamMatchingRunResult }

type samMatchingCandidatesInput struct {
	Status string `query:"status" default:""`
	Limit  int32  `query:"limit" minimum:"1" maximum:"100" default:"50"`
	Offset int32  `query:"offset" minimum:"0" default:"0"`
}

type samMatchingCandidatesOutput struct{ Body []SamMatchingCandidate }

type samMatchingSummaryOutput struct{ Body SamMatchingSummary }

type samMatchingCandidateInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type samMatchingCandidateOutput struct{ Body SamMatchingCandidate }

func Register(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID:  "sam-matching-run",
		Method:       http.MethodPost,
		Path:         "/api/sam/matching/run",
		Summary:      "Построить очередь сопоставления несопоставленных установок",
		Tags:         []string{"SAM matching"},
		MaxBodyBytes: 8192,
	}, func(ctx context.Context, in *samMatchingRunInput) (*samMatchingRunOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		result, err := s.Run(ctx, principal.Session.Username, in.Body.Limit)
		if err != nil {
			return nil, err
		}
		return &samMatchingRunOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-matching-candidates",
		Method:      http.MethodGet,
		Path:        "/api/sam/matching/candidates",
		Summary:     "Очередь кандидатов сопоставления",
		Tags:        []string{"SAM matching"},
	}, func(ctx context.Context, in *samMatchingCandidatesInput) (*samMatchingCandidatesOutput, error) {
		items, err := s.Candidates(ctx, in.Status, in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		return &samMatchingCandidatesOutput{Body: items}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-matching-summary",
		Method:      http.MethodGet,
		Path:        "/api/sam/matching/summary",
		Summary:     "Счётчики очереди сопоставления",
		Tags:        []string{"SAM matching"},
	}, func(ctx context.Context, _ *struct{}) (*samMatchingSummaryOutput, error) {
		summary, err := s.Summary(ctx)
		if err != nil {
			return nil, err
		}
		return &samMatchingSummaryOutput{Body: summary}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-matching-confirm",
		Method:      http.MethodPost,
		Path:        "/api/sam/matching/candidates/{id}/confirm",
		Summary:     "Подтвердить сопоставление и создать алиас",
		Tags:        []string{"SAM matching"},
	}, func(ctx context.Context, in *samMatchingCandidateInput) (*samMatchingCandidateOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		candidate, err := s.Confirm(ctx, principal.Session.Username, in.ID)
		if err != nil {
			return nil, err
		}
		return &samMatchingCandidateOutput{Body: candidate}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-matching-reject",
		Method:      http.MethodPost,
		Path:        "/api/sam/matching/candidates/{id}/reject",
		Summary:     "Отклонить кандидата сопоставления",
		Tags:        []string{"SAM matching"},
	}, func(ctx context.Context, in *samMatchingCandidateInput) (*samMatchingCandidateOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		candidate, err := s.Reject(ctx, principal.Session.Username, in.ID)
		if err != nil {
			return nil, err
		}
		return &samMatchingCandidateOutput{Body: candidate}, nil
	})
}
