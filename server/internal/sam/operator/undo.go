package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
)

// Undo reverts the most recent applied action using the before/after snapshot
// captured at apply time. Older actions cannot be undone until a newer applied
// action is undone first, so the audit trail stays linear.
func (s *Service) Undo(ctx context.Context, actor string, id int64) (SamOperatorAction, error) {
	err := s.run(ctx, func(store Store) error {
		latest, err := store.LatestAppliedSamAIAction(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("operator.nothing_to_undo")
		}
		if err != nil {
			return err
		}
		if latest.ID != id {
			return huma.Error409Conflict("operator.undo_not_latest")
		}
		action, err := store.GetSamAIActionForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("operator.not_found")
		}
		if err != nil {
			return err
		}
		if action.Status != "applied" {
			return huma.Error409Conflict("operator.not_applied")
		}
		if _, err := undoAction(ctx, store, action); err != nil {
			return err
		}
		rows, err := store.UndoSamAIAction(ctx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return huma.Error409Conflict("operator.not_applied")
		}
		return s.audit(ctx, store, actor, "sam_operator.undo", fmt.Sprintf("sam_ai_action:%d", id))
	})
	if err != nil {
		var status huma.StatusError
		if errors.As(err, &status) {
			return SamOperatorAction{}, err
		}
		return SamOperatorAction{}, storageError(ctx, "undo SAM action", err)
	}
	row, err := s.store.GetSamAIAction(ctx, id)
	if err != nil {
		return SamOperatorAction{}, storageError(ctx, "reload SAM action", err)
	}
	return actionView(row), nil
}

func undoAction(ctx context.Context, store Store, action db.SamAiAction) (int, error) {
	switch action.Intent {
	case "match_aliases":
		var after matchAfter
		if err := json.Unmarshal(action.After, &after); err != nil {
			return 0, fmt.Errorf("decode match after state: %w", err)
		}
		aliasIDs := make([]int64, 0, len(after.Aliases))
		for _, alias := range after.Aliases {
			aliasIDs = append(aliasIDs, alias.ID)
		}
		if len(aliasIDs) > 0 {
			rows, err := store.SamDeleteAliases(ctx, aliasIDs)
			if err != nil {
				return 0, err
			}
			if rows != int64(len(aliasIDs)) {
				return 0, huma.Error409Conflict("operator.undo_conflict")
			}
		}
		installIDs := make([]int64, 0, len(after.Installations))
		for _, installation := range after.Installations {
			installIDs = append(installIDs, installation.ID)
		}
		if len(installIDs) > 0 {
			rows, err := store.SamClearInstallationProduct(ctx, installIDs)
			if err != nil {
				return 0, err
			}
			if rows != int64(len(installIDs)) {
				return 0, huma.Error409Conflict("operator.undo_conflict")
			}
		}
		return len(aliasIDs) + len(installIDs), nil
	case "set_status", "archive_product":
		var before productChange
		if err := json.Unmarshal(action.Before, &before); err != nil {
			return 0, fmt.Errorf("decode status before state: %w", err)
		}
		ids := make([]int64, 0, len(before.Products))
		statuses := make([]string, 0, len(before.Products))
		for _, product := range before.Products {
			ids = append(ids, product.ID)
			statuses = append(statuses, product.Status)
		}
		rows, err := store.SamRestoreProductStatuses(ctx, db.SamRestoreProductStatusesParams{Ids: ids, Statuses: statuses})
		if err != nil {
			return 0, err
		}
		if rows != int64(len(ids)) {
			return 0, huma.Error409Conflict("operator.undo_conflict")
		}
		return len(ids), nil
	case "set_category":
		var before productChange
		if err := json.Unmarshal(action.Before, &before); err != nil {
			return 0, fmt.Errorf("decode category before state: %w", err)
		}
		ids := make([]int64, 0, len(before.Products))
		categories := make([]string, 0, len(before.Products))
		for _, product := range before.Products {
			ids = append(ids, product.ID)
			categories = append(categories, product.Category)
		}
		rows, err := store.SamRestoreProductCategories(ctx, db.SamRestoreProductCategoriesParams{Ids: ids, Categories: categories})
		if err != nil {
			return 0, err
		}
		if rows != int64(len(ids)) {
			return 0, huma.Error409Conflict("operator.undo_conflict")
		}
		return len(ids), nil
	case "update_license":
		var before licenseChange
		if err := json.Unmarshal(action.Before, &before); err != nil {
			return 0, fmt.Errorf("decode license before state: %w", err)
		}
		for _, license := range before.Licenses {
			rows, err := store.SamUpdateLicense(ctx, db.SamUpdateLicenseParams{
				ID:          license.ID,
				LicenseType: license.LicenseType,
				Seats:       license.Seats,
				CostPerSeat: license.CostPerSeat,
				Currency:    license.Currency,
				StartsAt:    dateFromString(license.StartsAt),
				ExpiresAt:   dateFromString(license.ExpiresAt),
				Status:      license.Status,
			})
			if err != nil {
				return 0, err
			}
			if rows != 1 {
				return 0, huma.Error409Conflict("operator.undo_conflict")
			}
		}
		return len(before.Licenses), nil
	case "create_product":
		var after productCreated
		if err := json.Unmarshal(action.After, &after); err != nil {
			return 0, fmt.Errorf("decode product after state: %w", err)
		}
		aliasIDs := make([]int64, 0, len(after.Aliases))
		for _, alias := range after.Aliases {
			aliasIDs = append(aliasIDs, alias.ID)
		}
		if len(aliasIDs) > 0 {
			rows, err := store.SamDeleteAliases(ctx, aliasIDs)
			if err != nil {
				return 0, err
			}
			if rows != int64(len(aliasIDs)) {
				return 0, huma.Error409Conflict("operator.undo_conflict")
			}
		}
		rows, err := store.SamDeleteProduct(ctx, after.Product.ID)
		if err != nil {
			return 0, err
		}
		if rows != 1 {
			return 0, huma.Error409Conflict("operator.undo_conflict")
		}
		return 1 + len(aliasIDs), nil
	default:
		return 0, huma.Error409Conflict("operator.undo_unsupported")
	}
}
