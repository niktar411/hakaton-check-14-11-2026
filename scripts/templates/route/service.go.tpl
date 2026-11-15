package {{pkg}}

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotFound — записи нет или она скрыта от пользователя.
var ErrNotFound = errors.New("{{kebab}}: not found")

// Store — хранилище ресурса; реализуется методами sqlc из internal/db/sqlc.
// Реализация обязана возвращать ErrNotFound, когда записи нет.
type Store interface {
	Get{{Type}}(ctx context.Context, id int64) ({{Type}}, error)
}

// Service — логика и права ресурса. Handler не ходит в хранилище напрямую.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Get(ctx context.Context, id int64) ({{Type}}, error) {
	item, err := s.store.Get{{Type}}(ctx, id)
	if err != nil {
		return {{Type}}{}, fmt.Errorf("get {{kebab}} %d: %w", id, err)
	}
	return item, nil
}
