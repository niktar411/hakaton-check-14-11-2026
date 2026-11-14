package notifications

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

type fakeStore struct {
	mu sync.Mutex

	accounts    []db.ListEnabledLocalAccountsRow
	insertErr   error
	listErr     error
	listEnabled bool

	rows        map[int64]fakeRow
	nextID      int64
	dedup       map[string]bool
	insertCalls int
}

type fakeRow struct {
	userID   int64
	kind     string
	dedupKey string
	title    string
	body     string
	acked    bool
	ackedN   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		accounts: []db.ListEnabledLocalAccountsRow{
			{ID: 1, Username: "niktar411", DisplayName: "Никита"},
			{ID: 2, Username: "kvant", DisplayName: "Леонид"},
			{ID: 3, Username: "hensy", DisplayName: "Егор"},
		},
		rows:  map[int64]fakeRow{},
		dedup: map[string]bool{},
	}
}

func (f *fakeStore) ListEnabledLocalAccounts(context.Context) ([]db.ListEnabledLocalAccountsRow, error) {
	if f.listEnabled {
		return nil, context.DeadlineExceeded
	}
	return f.accounts, nil
}

func (f *fakeStore) CreateChatNotification(_ context.Context, arg db.CreateChatNotificationParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.insertCalls++
	if f.insertErr != nil {
		return 0, f.insertErr
	}
	key := fmt.Sprintf("%d|%s", arg.UserID, arg.DedupKey)
	if f.dedup[key] {
		return 0, nil
	}
	f.dedup[key] = true
	f.nextID++
	f.rows[f.nextID] = fakeRow{userID: arg.UserID, kind: arg.Kind, dedupKey: arg.DedupKey, title: arg.Title, body: arg.Body}
	return 1, nil
}

func (f *fakeStore) ListChatNotifications(_ context.Context, arg db.ListChatNotificationsParams) ([]db.ListChatNotificationsRow, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	userID := f.userID(arg.Username)
	result := []db.ListChatNotificationsRow{}
	for id := f.nextID; id >= 1; id-- {
		data, ok := f.rows[id]
		if !ok || data.userID != userID || !f.dedup[fmt.Sprintf("%d|%s", userID, data.dedupKey)] {
			continue
		}
		result = append(result, db.ListChatNotificationsRow{
			ID:             id,
			Kind:           data.kind,
			Title:          data.title,
			Body:           data.body,
			AcknowledgedAt: pgTimestamptz(data.acked),
		})
	}
	return result, nil
}

func (f *fakeStore) AcknowledgeChatNotification(_ context.Context, arg db.AcknowledgeChatNotificationParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	userID := f.userID(arg.Username)
	data, ok := f.rows[arg.ID]
	if !ok || data.userID != userID || !f.dedup[fmt.Sprintf("%d|%s", userID, data.dedupKey)] || data.acked {
		return 0, nil
	}
	data.acked = true
	data.ackedN++
	f.rows[arg.ID] = data
	return 1, nil
}

func (f *fakeStore) userID(username string) int64 {
	for _, account := range f.accounts {
		if account.Username == username {
			return account.ID
		}
	}
	return 0
}

func pgTimestamptz(set bool) pgtype.Timestamptz {
	if !set {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: time.Now(), Valid: true}
}

func TestFanOutDeliversOncePerAccount(t *testing.T) {
	store := newFakeStore()
	service := New(store)
	service.fanOut(context.Background(), KindBlockCompleted, "block_done:19", "Блок завершён: Smart Matching", "тело")
	service.fanOut(context.Background(), KindBlockCompleted, "block_done:19", "Блок завершён: Smart Matching", "тело")

	if store.insertCalls != len(store.accounts) {
		t.Fatalf("insert calls = %d, want %d (one per account)", store.insertCalls, len(store.accounts))
	}
	if len(store.rows) != len(store.accounts) {
		t.Fatalf("rows = %d, want %d", len(store.rows), len(store.accounts))
	}
	for _, account := range store.accounts {
		if !store.dedup[fmt.Sprintf("%d|%s", account.ID, "block_done:19")] {
			t.Fatalf("account %s did not receive the event", account.Username)
		}
	}
}

func TestFanOutRetriesAfterDeliveryFailure(t *testing.T) {
	store := newFakeStore()
	store.insertErr = context.DeadlineExceeded
	service := New(store)
	service.fanOut(context.Background(), KindBlockCompleted, "block_done:20", "Блок завершён", "тело")
	if len(store.rows) != 0 {
		t.Fatalf("failed pass must not create rows, got %d", len(store.rows))
	}
	store.insertErr = nil
	service.fanOut(context.Background(), KindBlockCompleted, "block_done:20", "Блок завершён", "тело")
	if len(store.rows) != len(store.accounts) {
		t.Fatalf("rows after retry = %d, want %d", len(store.rows), len(store.accounts))
	}
}

func TestListIsScopedToUserAndMappedToAPIType(t *testing.T) {
	store := newFakeStore()
	service := New(store)
	service.fanOut(context.Background(), KindMergeRequestCreated, "mr_created:7", "MR создан", "тело 7")

	rows, err := service.List(context.Background(), "kvant")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Kind != KindMergeRequestCreated || rows[0].Title != "MR создан" {
		t.Fatalf("unexpected notification: %+v", rows[0])
	}
	if rows[0].AcknowledgedAt != nil {
		t.Fatalf("fresh notification must be unacknowledged")
	}

	other, err := service.List(context.Background(), "hensy")
	if err != nil {
		t.Fatalf("list other: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("other user must see their own copy, got %d", len(other))
	}
}

func TestAcknowledgeIsIdempotentAndScoped(t *testing.T) {
	store := newFakeStore()
	service := New(store)
	service.fanOut(context.Background(), KindBlockCompleted, "block_done:27", "Блок завершён", "тело")

	rows, err := service.List(context.Background(), "niktar411")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	id := rows[0].ID
	if err := service.Acknowledge(context.Background(), "niktar411", id); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	// Second attempt and a foreign user are silent no-ops.
	if err := service.Acknowledge(context.Background(), "niktar411", id); err != nil {
		t.Fatalf("second acknowledge must not fail: %v", err)
	}
	if err := service.Acknowledge(context.Background(), "kvant", id); err != nil {
		t.Fatalf("foreign acknowledge must not fail: %v", err)
	}
	if store.rows[id].ackedN != 1 {
		t.Fatalf("acknowledge count = %d, want 1", store.rows[id].ackedN)
	}
	if err := service.Acknowledge(context.Background(), "kvant", 9999); err != nil {
		t.Fatalf("unknown id acknowledge must not fail: %v", err)
	}
}
