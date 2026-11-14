package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
)

const (
	defaultRunLimit = 1000
	maxRunLimit     = 5000
	maxCandidates   = 3
)

type Store interface {
	SamMatchingUnmatchedInstallations(ctx context.Context, arg db.SamMatchingUnmatchedInstallationsParams) ([]db.SamMatchingUnmatchedInstallationsRow, error)
	SamMatchingProducts(ctx context.Context) ([]db.SamMatchingProductsRow, error)
	SamMatchingAliases(ctx context.Context) ([]db.SamMatchingAliasesRow, error)
	SamMatchingUpsertCandidate(ctx context.Context, arg db.SamMatchingUpsertCandidateParams) (db.SamMatchCandidate, error)
	SamMatchingCandidates(ctx context.Context, arg db.SamMatchingCandidatesParams) ([]db.SamMatchingCandidatesRow, error)
	SamMatchingCandidateView(ctx context.Context, id int64) (db.SamMatchingCandidateViewRow, error)
	SamMatchingCandidateForUpdate(ctx context.Context, id int64) (db.SamMatchCandidate, error)
	SamMatchingConfirmCandidate(ctx context.Context, arg db.SamMatchingConfirmCandidateParams) (int64, error)
	SamMatchingRejectCandidate(ctx context.Context, arg db.SamMatchingRejectCandidateParams) (int64, error)
	SamMatchingRejectOtherCandidates(ctx context.Context, arg db.SamMatchingRejectOtherCandidatesParams) (int64, error)
	SamMatchingAliasProduct(ctx context.Context, normalizedAlias string) (int64, error)
	SamMatchingUpsertAlias(ctx context.Context, arg db.SamMatchingUpsertAliasParams) (int64, error)
	SamMatchingSetInstallationProduct(ctx context.Context, arg db.SamMatchingSetInstallationProductParams) (int64, error)
	SamMatchingCountUnmatched(ctx context.Context) (int64, error)
	SamMatchingCountPending(ctx context.Context) (int64, error)
	SamMatchingCountCandidatesByStatus(ctx context.Context, status string) (int64, error)
	BeginAudit(ctx context.Context, arg db.BeginAuditParams) (int64, error)
	FinishAudit(ctx context.Context, arg db.FinishAuditParams) error
}

type TxRunner func(ctx context.Context, fn func(Store) error) error

type Service struct {
	store Store
	run   TxRunner
}

func NewService(pool *pgxpool.Pool) *Service {
	queries := db.New(pool)
	run := func(ctx context.Context, fn func(Store) error) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin SAM matching transaction: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				if err := tx.Rollback(context.WithoutCancel(ctx)); err != nil {
					slog.WarnContext(ctx, "rollback SAM matching transaction failed", "error", err)
				}
			}
		}()
		if err := fn(queries.WithTx(tx)); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit SAM matching transaction: %w", err)
		}
		committed = true
		return nil
	}
	return newService(queries, run)
}

func newService(store Store, run TxRunner) *Service {
	return &Service{store: store, run: run}
}

// Run proposes candidates for unmatched installations. It never changes
// sam_installations or sam_product_aliases: confirmation is a separate step.
func (s *Service) Run(ctx context.Context, actor string, limit int32) (SamMatchingRunResult, error) {
	if actor == "" {
		return SamMatchingRunResult{}, huma.Error401Unauthorized("auth.required")
	}
	if limit <= 0 {
		limit = defaultRunLimit
	}
	if limit > maxRunLimit {
		limit = maxRunLimit
	}
	result := SamMatchingRunResult{Limit: limit}
	err := s.run(ctx, func(store Store) error {
		unmatched, err := store.SamMatchingCountUnmatched(ctx)
		if err != nil {
			return err
		}
		result.Unmatched = unmatched
		installations, err := store.SamMatchingUnmatchedInstallations(ctx, db.SamMatchingUnmatchedInstallationsParams{Limit: limit, Offset: 0})
		if err != nil {
			return err
		}
		products, err := store.SamMatchingProducts(ctx)
		if err != nil {
			return err
		}
		aliases, err := store.SamMatchingAliases(ctx)
		if err != nil {
			return err
		}
		index := BuildIndex(catalogProducts(products), catalogAliases(aliases))
		result.Scanned = int64(len(installations))
		for _, installation := range installations {
			matches := BestMatches(installation.RawName, index, maxCandidates)
			if len(matches) == 0 {
				result.Skipped++
				continue
			}
			proposed := false
			for _, match := range matches {
				_, err := store.SamMatchingUpsertCandidate(ctx, db.SamMatchingUpsertCandidateParams{
					InstallationID: installation.ID,
					ProductID:      match.ProductID,
					RawName:        installation.RawName,
					Score:          float32(match.Score),
					Method:         match.Method,
				})
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				if err != nil {
					return err
				}
				result.Candidates++
				proposed = true
			}
			if proposed {
				result.Matched++
			} else {
				result.Skipped++
			}
		}
		result.Pending, err = store.SamMatchingCountPending(ctx)
		return err
	})
	if err != nil {
		return SamMatchingRunResult{}, storageError(ctx, "run SAM matching", err)
	}
	return result, nil
}

func (s *Service) Candidates(ctx context.Context, status string, limit, offset int32) ([]SamMatchingCandidate, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, huma.Error422UnprocessableEntity("matching.invalid_pagination")
	}
	switch status {
	case "", "pending", "confirmed", "rejected":
	default:
		return nil, huma.Error422UnprocessableEntity("matching.invalid_status")
	}
	rows, err := s.store.SamMatchingCandidates(ctx, db.SamMatchingCandidatesParams{Status: status, Limit: limit, Offset: offset})
	if err != nil {
		return nil, storageError(ctx, "list SAM match candidates", err)
	}
	result := make([]SamMatchingCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, candidateFromRow(row))
	}
	return result, nil
}

func (s *Service) Summary(ctx context.Context) (SamMatchingSummary, error) {
	unmatched, err := s.store.SamMatchingCountUnmatched(ctx)
	if err != nil {
		return SamMatchingSummary{}, storageError(ctx, "count unmatched installations", err)
	}
	pending, err := s.store.SamMatchingCountCandidatesByStatus(ctx, "pending")
	if err != nil {
		return SamMatchingSummary{}, storageError(ctx, "count pending candidates", err)
	}
	confirmed, err := s.store.SamMatchingCountCandidatesByStatus(ctx, "confirmed")
	if err != nil {
		return SamMatchingSummary{}, storageError(ctx, "count confirmed candidates", err)
	}
	rejected, err := s.store.SamMatchingCountCandidatesByStatus(ctx, "rejected")
	if err != nil {
		return SamMatchingSummary{}, storageError(ctx, "count rejected candidates", err)
	}
	return SamMatchingSummary{Unmatched: unmatched, Pending: pending, Confirmed: confirmed, Rejected: rejected}, nil
}

func (s *Service) Confirm(ctx context.Context, actor string, id int64) (SamMatchingCandidate, error) {
	if actor == "" {
		return SamMatchingCandidate{}, huma.Error401Unauthorized("auth.required")
	}
	err := s.run(ctx, func(store Store) error {
		candidate, err := store.SamMatchingCandidateForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("matching.not_found")
		}
		if err != nil {
			return err
		}
		if candidate.Status != "pending" {
			return huma.Error409Conflict("matching.not_pending")
		}
		normalized := Normalize(candidate.RawName)
		if normalized == "" {
			return huma.Error422UnprocessableEntity("matching.empty_alias")
		}
		owner, err := store.SamMatchingAliasProduct(ctx, normalized)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && owner != candidate.ProductID {
			return huma.Error409Conflict("matching.alias_conflict")
		}
		rows, err := store.SamMatchingUpsertAlias(ctx, db.SamMatchingUpsertAliasParams{
			ProductID:       candidate.ProductID,
			Alias:           candidate.RawName,
			NormalizedAlias: normalized,
			CreatedBy:       actor,
			Confidence:      1,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return huma.Error409Conflict("matching.alias_conflict")
		}
		rows, err = store.SamMatchingSetInstallationProduct(ctx, db.SamMatchingSetInstallationProductParams{
			ID:        candidate.InstallationID,
			ProductID: pgtype.Int8{Int64: candidate.ProductID, Valid: true},
		})
		if err != nil {
			return err
		}
		if rows != 1 {
			return huma.Error409Conflict("matching.installation_changed")
		}
		rows, err = store.SamMatchingConfirmCandidate(ctx, db.SamMatchingConfirmCandidateParams{ID: id, ResolvedBy: text(actor)})
		if err != nil {
			return err
		}
		if rows != 1 {
			return huma.Error409Conflict("matching.not_pending")
		}
		if _, err := store.SamMatchingRejectOtherCandidates(ctx, db.SamMatchingRejectOtherCandidatesParams{
			InstallationID: candidate.InstallationID,
			ResolvedBy:     text(actor),
			ID:             id,
		}); err != nil {
			return err
		}
		return s.audit(ctx, store, actor, "sam_matching.confirm", fmt.Sprintf("sam_match_candidate:%d", id))
	})
	if err != nil {
		return SamMatchingCandidate{}, matchError(ctx, "confirm SAM match candidate", err)
	}
	return s.candidate(ctx, id)
}

func (s *Service) Reject(ctx context.Context, actor string, id int64) (SamMatchingCandidate, error) {
	if actor == "" {
		return SamMatchingCandidate{}, huma.Error401Unauthorized("auth.required")
	}
	err := s.run(ctx, func(store Store) error {
		candidate, err := store.SamMatchingCandidateForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("matching.not_found")
		}
		if err != nil {
			return err
		}
		if candidate.Status != "pending" {
			return huma.Error409Conflict("matching.not_pending")
		}
		rows, err := store.SamMatchingRejectCandidate(ctx, db.SamMatchingRejectCandidateParams{ID: id, ResolvedBy: text(actor)})
		if err != nil {
			return err
		}
		if rows != 1 {
			return huma.Error409Conflict("matching.not_pending")
		}
		return s.audit(ctx, store, actor, "sam_matching.reject", fmt.Sprintf("sam_match_candidate:%d", id))
	})
	if err != nil {
		return SamMatchingCandidate{}, matchError(ctx, "reject SAM match candidate", err)
	}
	return s.candidate(ctx, id)
}

func (s *Service) candidate(ctx context.Context, id int64) (SamMatchingCandidate, error) {
	row, err := s.store.SamMatchingCandidateView(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return SamMatchingCandidate{}, huma.Error404NotFound("matching.not_found")
	}
	if err != nil {
		return SamMatchingCandidate{}, storageError(ctx, "load SAM match candidate", err)
	}
	return SamMatchingCandidate{
		ID:             row.ID,
		InstallationID: row.InstallationID,
		ProductID:      row.ProductID,
		RawName:        row.RawName,
		ProductName:    row.ProductName,
		DeviceName:     row.DeviceName,
		Score:          row.Score,
		Method:         row.Method,
		Status:         row.Status,
		CreatedAt:      row.CreatedAt.Time,
		ResolvedAt:     timePointer(row.ResolvedAt),
		ResolvedBy:     row.ResolvedBy.String,
		LastSeenAt:     row.LastSeenAt.Time,
	}, nil
}

func (s *Service) audit(ctx context.Context, store Store, actor, action, target string) error {
	id, err := store.BeginAudit(ctx, db.BeginAuditParams{Username: actor, Action: action, Target: target})
	if err != nil {
		return err
	}
	return store.FinishAudit(ctx, db.FinishAuditParams{ID: id, Status: "completed"})
}

func candidateFromRow(row db.SamMatchingCandidatesRow) SamMatchingCandidate {
	return SamMatchingCandidate{
		ID:             row.ID,
		InstallationID: row.InstallationID,
		ProductID:      row.ProductID,
		RawName:        row.RawName,
		ProductName:    row.ProductName,
		DeviceName:     row.DeviceName,
		Score:          row.Score,
		Method:         row.Method,
		Status:         row.Status,
		CreatedAt:      row.CreatedAt.Time,
		ResolvedAt:     timePointer(row.ResolvedAt),
		ResolvedBy:     row.ResolvedBy.String,
		LastSeenAt:     row.LastSeenAt.Time,
	}
}

func catalogProducts(rows []db.SamMatchingProductsRow) []CatalogProduct {
	result := make([]CatalogProduct, 0, len(rows))
	for _, row := range rows {
		result = append(result, CatalogProduct{ID: row.ID, Name: row.Name})
	}
	return result
}

func catalogAliases(rows []db.SamMatchingAliasesRow) []CatalogAlias {
	result := make([]CatalogAlias, 0, len(rows))
	for _, row := range rows {
		result = append(result, CatalogAlias{ProductID: row.ProductID, Alias: row.Alias, Confidence: row.Confidence})
	}
	return result
}

func text(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func matchError(ctx context.Context, operation string, err error) error {
	var status huma.StatusError
	if errors.As(err, &status) {
		return err
	}
	return storageError(ctx, operation, err)
}

func storageError(ctx context.Context, operation string, err error) error {
	wrapped := fmt.Errorf("%s: %w", operation, err)
	slog.ErrorContext(ctx, "SAM matching storage operation failed", "error", wrapped)
	return huma.Error503ServiceUnavailable("storage.unavailable")
}
