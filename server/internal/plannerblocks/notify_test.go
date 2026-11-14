package plannerblocks

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
)

type recordingNotifier struct {
	mu      sync.Mutex
	blocks  []int64
	titles  []string
	owners  []string
	mrCalls []int64
}

func (r *recordingNotifier) BlockCompleted(_ context.Context, blockID int64, title, ownerUsername string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blocks = append(r.blocks, blockID)
	r.titles = append(r.titles, title)
	r.owners = append(r.owners, ownerUsername)
}

func (r *recordingNotifier) MergeRequestCreated(_ context.Context, taskID int64, _, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mrCalls = append(r.mrCalls, taskID)
}

func TestListNotifiesWhenMergeRequestIsMerged(t *testing.T) {
	store := newFakeStore()
	store.blocks[7] = db.PlannerBlock{
		ID: 7, Title: "SAM block 19: Smart Matching", OwnerID: 1,
		MergeRequestIid: pgtype.Int8{Int64: 42, Valid: true},
	}
	gitLab := &fakeGitLab{mergeRequests: map[int64]gitlab.MergeRequest{
		42: {IID: 42, State: "merged", WebURL: "https://gitlab.example/mr/42"},
	}}
	notifier := &recordingNotifier{}
	service := newService(store, gitLab, testConfig)
	service.SetNotifier(notifier)

	if _, err := service.List(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	notifier.mu.Lock()
	calls := len(notifier.blocks)
	notifier.mu.Unlock()
	if calls != 1 {
		t.Fatalf("BlockCompleted calls = %d, want 1", calls)
	}
	if notifier.blocks[0] != 7 || notifier.titles[0] != "SAM block 19: Smart Matching" || notifier.owners[0] != "alice" {
		t.Fatalf("unexpected notification payload: %+v", notifier)
	}
}

func TestListDoesNotNotifyForOpenMergeRequest(t *testing.T) {
	store := newFakeStore()
	store.blocks[7] = db.PlannerBlock{
		ID: 7, Title: "SAM block 19: Smart Matching", OwnerID: 1,
		MergeRequestIid: pgtype.Int8{Int64: 42, Valid: true},
	}
	gitLab := &fakeGitLab{mergeRequests: map[int64]gitlab.MergeRequest{
		42: {IID: 42, State: "opened", WebURL: "https://gitlab.example/mr/42", HeadPipeline: gitlab.Pipeline{Status: "success"}},
	}}
	notifier := &recordingNotifier{}
	service := newService(store, gitLab, testConfig)
	service.SetNotifier(notifier)

	if _, err := service.List(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(notifier.blocks) != 0 {
		t.Fatalf("open merge request must not notify, got %d calls", len(notifier.blocks))
	}
}
