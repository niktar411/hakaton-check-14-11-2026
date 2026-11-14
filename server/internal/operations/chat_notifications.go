package operations

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"hackathon/internal/identity"
)

// ChatNotifications returns the calling user's recent team events: completed
// planner blocks and merge requests created by coding tasks.
func (s *Service) ChatNotifications(ctx context.Context, limit, offset int32) ([]ChatNotification, error) {
	principal, err := identity.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.notifications.List(ctx, principal.Session.Username)
	if err != nil {
		slog.ErrorContext(ctx, "chat notifications list failed", "error", err)
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	if offset > 0 && int(offset) < len(rows) {
		rows = rows[offset:]
	} else if offset > 0 {
		rows = nil
	}
	if limit > 0 && int(limit) < len(rows) {
		rows = rows[:limit]
	}
	result := make([]ChatNotification, 0, len(rows))
	for _, row := range rows {
		result = append(result, ChatNotification{
			ID:             row.ID,
			Kind:           row.Kind,
			Title:          row.Title,
			Body:           row.Body,
			CreatedAt:      row.CreatedAt,
			AcknowledgedAt: row.AcknowledgedAt,
		})
	}
	return result, nil
}

// AcknowledgeChatNotification marks one of the user's own notifications as
// seen. It is idempotent and never reveals whether the id belongs to someone
// else: a foreign or unknown id is a silent no-op.
func (s *Service) AcknowledgeChatNotification(ctx context.Context, id int64) (*chatNotificationAckOutput, error) {
	principal, err := identity.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.notifications.Acknowledge(ctx, principal.Session.Username, id); err != nil {
		slog.ErrorContext(ctx, "chat notification acknowledge failed", "id", id, "error", err)
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	return &chatNotificationAckOutput{}, nil
}
