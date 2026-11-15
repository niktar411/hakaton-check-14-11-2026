package deployments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
)

const webhookBodyLimit = 64 * 1024

type gitLabWebhookEvent struct {
	ObjectKind       string `json:"object_kind"`
	ObjectAttributes struct {
		IID int64 `json:"iid"`
	} `json:"object_attributes"`
}

type giteaWebhookEvent struct {
	Action      string `json:"action"`
	Number      int64  `json:"number"`
	PullRequest struct {
		Number int64 `json:"number"`
		Merged bool  `json:"merged"`
		Base   struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
}

// HandleGitLabWebhook is mounted by the operations router outside the
// authenticated Huma group. Only the IID is taken from the webhook payload; all
// security-sensitive merge fields are independently reloaded from GitLab.
func (s *Service) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeWebhookError(ctx, w, http.StatusMethodNotAllowed, "deployment.webhook_method")
		return
	}
	var iid int64
	if strings.EqualFold(r.Header.Get("X-Gitea-Event"), "pull_request") {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBodyLimit))
		if err != nil {
			writeWebhookError(ctx, w, http.StatusBadRequest, "deployment.webhook_invalid")
			return
		}
		if !webhookSignatureEqual(r.Header.Get("X-Gitea-Signature"), body, s.config.GitLabWebhookSecret) {
			writeWebhookError(ctx, w, http.StatusForbidden, "deployment.webhook_forbidden")
			return
		}
		var event giteaWebhookEvent
		if err := json.Unmarshal(body, &event); err != nil {
			writeWebhookError(ctx, w, http.StatusBadRequest, "deployment.webhook_invalid")
			return
		}
		if event.Action != "closed" || !event.PullRequest.Merged || event.PullRequest.Base.Ref != "main" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		iid = event.Number
		if iid < 1 {
			iid = event.PullRequest.Number
		}
		if iid < 1 {
			writeWebhookError(ctx, w, http.StatusBadRequest, "deployment.webhook_invalid")
			return
		}
	} else {
		if !webhookTokenEqual(r.Header.Get("X-Gitlab-Token"), s.config.GitLabWebhookSecret) {
			writeWebhookError(ctx, w, http.StatusForbidden, "deployment.webhook_forbidden")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, webhookBodyLimit)
		decoder := json.NewDecoder(r.Body)
		var event gitLabWebhookEvent
		if err := decoder.Decode(&event); err != nil {
			writeWebhookError(ctx, w, http.StatusBadRequest, "deployment.webhook_invalid")
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			writeWebhookError(ctx, w, http.StatusBadRequest, "deployment.webhook_invalid")
			return
		}
		if event.ObjectKind != "merge_request" || event.ObjectAttributes.IID < 1 {
			writeWebhookError(ctx, w, http.StatusBadRequest, "deployment.webhook_invalid")
			return
		}
		iid = event.ObjectAttributes.IID
	}
	mr, err := s.gitLab.MergeRequestByIID(ctx, iid)
	if err != nil {
		slog.ErrorContext(ctx, "reload webhook merge request failed", "iid", iid, "error", err)
		writeWebhookError(ctx, w, http.StatusBadGateway, "gitlab.merge_request_unavailable")
		return
	}
	if mr.State != "merged" || mr.TargetBranch != "main" || mr.SHA == "" || !validRelease(mr.SHA) {
		writeWebhookError(ctx, w, http.StatusConflict, "deployment.webhook_merge_invalid")
		return
	}
	approval, err := s.store.GetMergeApproval(ctx, db.GetMergeApprovalParams{MergeRequestIid: iid, CommitSha: mr.SHA})
	if errors.Is(err, pgx.ErrNoRows) {
		writeWebhookError(ctx, w, http.StatusForbidden, "deployment.approval_required")
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "load webhook merge approval failed", "iid", iid, "error", err)
		writeWebhookError(ctx, w, http.StatusServiceUnavailable, "storage.unavailable")
		return
	}
	if approval.MergeRequestIid != iid || approval.CommitSha != mr.SHA ||
		approval.SourceBranch != mr.SourceBranch || approval.TargetBranch != "main" ||
		approval.ApprovedBy == "" || approval.ApprovedBy != s.config.AdminUser {
		writeWebhookError(ctx, w, http.StatusForbidden, "deployment.approval_required")
		return
	}
	if !pipelineSuccessful(mr) {
		writeWebhookError(ctx, w, http.StatusConflict, "deployment.pipeline_not_successful")
		return
	}

	operation, err := s.execute(ctx, approval.ApprovedBy, ActionDeploy, TargetProduction, mr.SHA)
	if err != nil {
		writeWebhookServiceError(ctx, w, err)
		return
	}
	writeWebhookJSON(ctx, w, http.StatusOK, operation)
}

func webhookTokenEqual(provided, configured string) bool {
	if configured == "" {
		return false
	}
	providedDigest := sha256.Sum256([]byte(provided))
	configuredDigest := sha256.Sum256([]byte(configured))
	return subtle.ConstantTimeCompare(providedDigest[:], configuredDigest[:]) == 1
}

func webhookSignatureEqual(provided string, body []byte, secret string) bool {
	if provided == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(body); err != nil {
		return false
	}
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(strings.ToLower(provided))) == 1
}

func writeWebhookServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var statusError interface{ GetStatus() int }
	if errors.As(err, &statusError) {
		candidate := statusError.GetStatus()
		if candidate >= 400 && candidate <= 599 {
			status = candidate
		}
	}
	// execute returns only fixed Huma errors. Keep a generic fallback for any
	// future error implementation so internal errors cannot become public.
	message := "deployment.failed_check_state"
	if statusError != nil {
		message = err.Error()
	}
	writeWebhookError(ctx, w, status, message)
}

func writeWebhookError(ctx context.Context, w http.ResponseWriter, status int, code string) {
	writeWebhookJSON(ctx, w, status, struct {
		Error string `json:"error"`
	}{Error: code})
}

func writeWebhookJSON(ctx context.Context, w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		payload = []byte(`{"error":"deployment.response_failed"}`)
	}
	w.WriteHeader(status)
	if _, err := w.Write(payload); err != nil {
		slog.WarnContext(ctx, "write GitLab webhook response failed", "error", err)
	}
}
