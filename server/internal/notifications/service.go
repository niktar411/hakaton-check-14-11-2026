// Package notifications records team events in the chat. Every enabled
// account receives its own copy in chat_notifications; the unique
// (user_id, dedup_key) pair is the deduplication authority, so replaying an
// event never creates a duplicate row.
package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
)

// Store is the subset of db.Queries used by the service.
type Store interface {
	ListChatNotifications(ctx context.Context, arg db.ListChatNotificationsParams) ([]db.ListChatNotificationsRow, error)
	CreateChatNotification(ctx context.Context, arg db.CreateChatNotificationParams) (int64, error)
	AcknowledgeChatNotification(ctx context.Context, arg db.AcknowledgeChatNotificationParams) (int64, error)
	ListEnabledLocalAccounts(ctx context.Context) ([]db.ListEnabledLocalAccountsRow, error)
}

// Notifier is what producers (planner, coding tasks) call. Calls are
// fire-and-forget: the event is recorded with a detached context so a
// finishing request cannot cancel delivery.
type Notifier interface {
	BlockCompleted(ctx context.Context, blockID int64, title, ownerUsername string)
	MergeRequestCreated(ctx context.Context, taskID int64, branch, mergeRequestURL string)
}

// Service implements Notifier plus the read/acknowledge API used by chat.
type Service struct {
	store Store
	mu    sync.Mutex
	// seen keeps the dedup keys already fanned out in this process. The
	// database unique constraint stays authoritative across restarts; the
	// map only avoids repeating known inserts.
	seen map[string]bool
}

// NewService builds the service over a connection pool.
func NewService(pool *pgxpool.Pool) *Service {
	return New(db.New(pool))
}

// New builds the service over a store, for tests.
func New(store Store) *Service {
	return &Service{store: store, seen: map[string]bool{}}
}

// BlockCompleted records one block_completed copy for every enabled account.
// Delivery is asynchronous: the caller's context may be a request that is
// about to end, so the fan-out runs on a detached context.
func (s *Service) BlockCompleted(ctx context.Context, blockID int64, title, ownerUsername string) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = fmt.Sprintf("Блок #%d", blockID)
	}
	body := fmt.Sprintf("Ответственный: %s. Merge request влит, блок отмечен выполненным в плане.", ownerUsername)
	eventTitle := fmt.Sprintf("Блок завершён: %s", title)
	dedupKey := fmt.Sprintf("block_done:%d", blockID)
	go s.fanOut(ctx, KindBlockCompleted, dedupKey, eventTitle, body)
}

// MergeRequestCreated records one merge_request_created copy for every enabled
// account. Delivery is asynchronous, see BlockCompleted.
func (s *Service) MergeRequestCreated(ctx context.Context, taskID int64, branch, mergeRequestURL string) {
	dedupKey := fmt.Sprintf("mr_created:%d", taskID)
	eventTitle := "Coding-задача создала merge request"
	body := fmt.Sprintf("Ветка %s готова к ревью: %s", branch, mergeRequestURL)
	go s.fanOut(ctx, KindMergeRequestCreated, dedupKey, eventTitle, body)
}

// fanOut delivers one event to every enabled account exactly once.
func (s *Service) fanOut(ctx context.Context, kind, dedupKey, title, body string) {
	deliveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	s.mu.Lock()
	if s.seen[dedupKey] {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	accounts, err := s.store.ListEnabledLocalAccounts(deliveryCtx)
	if err != nil {
		// The key is intentionally not marked seen on failure so the next
		// replay can retry delivery.
		slog.ErrorContext(deliveryCtx, "chat notification recipients unavailable", "kind", kind, "dedup_key", dedupKey, "error", err)
		return
	}
	failed := 0
	for _, account := range accounts {
		if _, err := s.store.CreateChatNotification(deliveryCtx, db.CreateChatNotificationParams{
			UserID:   account.ID,
			Kind:     kind,
			DedupKey: dedupKey,
			Title:    title,
			Body:     body,
		}); err != nil {
			failed++
			slog.ErrorContext(deliveryCtx, "chat notification insert failed", "kind", kind, "dedup_key", dedupKey, "account_id", account.ID, "error", err)
		}
	}
	// Mark seen only after a fully delivered pass; a failed insert leaves the
	// key unseen so the next replay retries just the missing copies.
	if failed == 0 {
		s.mu.Lock()
		s.seen[dedupKey] = true
		s.mu.Unlock()
	}
}

// List returns the calling user's recent notifications, newest first.
func (s *Service) List(ctx context.Context, username string) ([]Notification, error) {
	rows, err := s.store.ListChatNotifications(ctx, db.ListChatNotificationsParams{
		Username: username,
		Limit:    listLimit,
		Offset:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("list chat notifications: %w", err)
	}
	result := make([]Notification, 0, len(rows))
	for _, row := range rows {
		notification := Notification{
			ID:        row.ID,
			Kind:      row.Kind,
			Title:     row.Title,
			Body:      row.Body,
			CreatedAt: row.CreatedAt.Time,
		}
		if row.AcknowledgedAt.Valid {
			at := row.AcknowledgedAt.Time
			notification.AcknowledgedAt = &at
		}
		result = append(result, notification)
	}
	return result, nil
}

// Acknowledge marks one of the user's own notifications acknowledged. It is
// idempotent: acknowledging twice is not an error and does not move the
// timestamp.
func (s *Service) Acknowledge(ctx context.Context, username string, id int64) error {
	updated, err := s.store.AcknowledgeChatNotification(ctx, db.AcknowledgeChatNotificationParams{ID: id, Username: username})
	if err != nil {
		return fmt.Errorf("acknowledge chat notification %d: %w", id, err)
	}
	if updated == 0 {
		// Either the notification belongs to another user or it is already
		// acknowledged. Never leak which one it was.
		return nil
	}
	return nil
}
