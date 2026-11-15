package sam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
	"hackathon/internal/sam/importer"
)

// ErrConflict marks duplicate names and other blocking collisions.
var ErrConflict = errors.New("sam: conflict")

// ErrValidation marks invalid field values; the handler maps it to 422.
var ErrValidation = errors.New("sam: invalid input")

// editRun runs fn inside one transaction so the edit and its audit row land
// together.
type editRun func(ctx context.Context, fn func(Store) error) error

func newEditRun(pool *pgxpool.Pool) editRun {
	return func(ctx context.Context, fn func(Store) error) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin SAM edit transaction: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				if err := tx.Rollback(context.WithoutCancel(ctx)); err != nil {
					slog.WarnContext(ctx, "rollback SAM edit transaction failed", "error", err)
				}
			}
		}()
		if err := fn(db.New(pool).WithTx(tx)); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit SAM edit transaction: %w", err)
		}
		committed = true
		return nil
	}
}

// requireEditor returns the acting account username. Any enabled local
// account (member or administrator) may edit SAM data; anonymous clients get
// 401, disabled or foreign sessions 403.
func requireEditor(ctx context.Context) (string, error) {
	principal, err := identity.FromContext(ctx)
	if err != nil {
		return "", err
	}
	if principal.AuthKind != "local" || !principal.Account.Enabled {
		return "", huma.Error403Forbidden("auth.forbidden")
	}
	return principal.Account.Username, nil
}

// auditEdit writes the audit row for one edit. before may be nil.
func auditEdit(ctx context.Context, edits Store, actor, action, entity, entityID string, before, after any) error {
	encode := func(value any) ([]byte, error) {
		if value == nil {
			return nil, nil
		}
		return json.Marshal(value)
	}
	beforeData, err := encode(before)
	if err != nil {
		return fmt.Errorf("encode SAM audit before: %w", err)
	}
	afterData, err := encode(after)
	if err != nil {
		return fmt.Errorf("encode SAM audit after: %w", err)
	}
	if err := edits.SamEditAuditInsert(ctx, db.SamEditAuditInsertParams{
		Actor:      actor,
		Action:     action,
		EntityType: entity,
		EntityID:   entityID,
		Before:     beforeData,
		After:      afterData,
	}); err != nil {
		return fmt.Errorf("write SAM audit row: %w", err)
	}
	return nil
}

func parseDate(raw *string) (pgtype.Date, error) {
	var parsed pgtype.Date
	if raw == nil || *raw == "" {
		return parsed, nil
	}
	day, err := time.Parse("2006-01-02", *raw)
	if err != nil {
		return parsed, fmt.Errorf("%w: valid_until must be YYYY-MM-DD", ErrValidation)
	}
	parsed = pgtype.Date{Time: day, Valid: true}
	return parsed, nil
}

func validateProductInput(input ProductInput) error {
	switch input.Status {
	case "active", "outdated", "banned", "archived":
	default:
		return fmt.Errorf("%w: product status", ErrValidation)
	}
	switch input.LicenseMetric {
	case "device", "user", "installation", "none":
	default:
		return fmt.Errorf("%w: license metric", ErrValidation)
	}
	return nil
}

func validateLicenseKind(kind string) error {
	switch kind {
	case "subscription", "perpetual", "open_source", "trial":
		return nil
	}
	return fmt.Errorf("%w: license kind", ErrValidation)
}

func validateInstallationStatus(status string) error {
	switch status {
	case "installed", "removed", "unknown":
		return nil
	}
	return fmt.Errorf("%w: installation status", ErrValidation)
}

func productParams(input ProductInput) db.SamEditCreateProductParams {
	if input.LicenseMetric == "" {
		input.LicenseMetric = "device"
	}
	if input.Status == "" {
		input.Status = "active"
	}
	return db.SamEditCreateProductParams{
		Name:          input.Name,
		Vendor:        input.Vendor,
		Version:       input.Version,
		Category:      input.Category,
		Status:        input.Status,
		SectionCode:   input.SectionCode,
		ClassCode:     input.ClassCode,
		LicenseMetric: input.LicenseMetric,
		RegistryRf:    input.RegistryRf,
		RegistryEntry: input.RegistryEntry,
		IsTrackable:   input.IsTrackable,
	}
}

// productFromEditRow maps a stored product row plus usage counts into the
// public Product model.
func productFromEditRow(row db.SamEditProductByIDRow, usage db.SamEditProductUsageRow) Product {
	return Product{
		ID:                row.ID,
		Name:              row.Name,
		Vendor:            row.Vendor,
		Version:           row.Version,
		Category:          row.Category,
		SectionCode:       row.SectionCode,
		ClassCode:         row.ClassCode,
		LicenseMetric:     row.LicenseMetric,
		RegistryRf:        row.RegistryRf,
		RegistryEntry:     row.RegistryEntry,
		IsTrackable:       row.IsTrackable,
		LicenseCount:      usage.LicenseCount,
		InstallationCount: usage.InstallationCount,
		UpdatedAt:         row.UpdatedAt.Time,
	}
}

// CreateProduct adds a catalog entry and writes an audit row.
func (s *Service) CreateProduct(ctx context.Context, input ProductInput) (Product, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return Product{}, err
	}
	if err := validateProductInput(input); err != nil {
		return Product{}, err
	}
	if exists, err := s.store.SamProductNameExists(ctx, input.Name); err == nil && exists {
		return Product{}, ErrConflict
	}
	var created Product
	err = s.editRun(ctx, func(edits Store) error {
		row, err := edits.SamEditCreateProduct(ctx, productParams(input))
		if err != nil {
			return fmt.Errorf("create SAM product: %w", err)
		}
		if err := auditEdit(ctx, edits, actor, "product.create", "product", fmt.Sprint(row.ID), nil, row); err != nil {
			return err
		}
		usage, err := edits.SamEditProductUsage(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("read SAM product usage: %w", err)
		}
		created = productFromEditRow(db.SamEditProductByIDRow(row), usage)
		return nil
	})
	if err != nil {
		return Product{}, err
	}
	return created, nil
}

// UpdateProduct replaces the editable fields of one catalog entry.
func (s *Service) UpdateProduct(ctx context.Context, id int64, input ProductInput) (Product, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return Product{}, err
	}
	if err := validateProductInput(input); err != nil {
		return Product{}, err
	}
	var updated Product
	err = s.editRun(ctx, func(edits Store) error {
		before, err := edits.SamEditProductByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("product.not_found")
		}
		if err != nil {
			return fmt.Errorf("read SAM product: %w", err)
		}
		if input.Name != before.Name {
			if exists, err := edits.SamProductNameExists(ctx, input.Name); err == nil && exists {
				return ErrConflict
			}
		}
		row, err := edits.SamEditUpdateProduct(ctx, db.SamEditUpdateProductParams{
			ID:            id,
			Name:          input.Name,
			Vendor:        input.Vendor,
			Version:       input.Version,
			Category:      input.Category,
			Status:        input.Status,
			SectionCode:   input.SectionCode,
			ClassCode:     input.ClassCode,
			LicenseMetric: input.LicenseMetric,
			RegistryRf:    input.RegistryRf,
			RegistryEntry: input.RegistryEntry,
			IsTrackable:   input.IsTrackable,
		})
		if err != nil {
			return fmt.Errorf("update SAM product: %w", err)
		}
		if err := auditEdit(ctx, edits, actor, "product.update", "product", fmt.Sprint(id), before, row); err != nil {
			return err
		}
		usage, err := edits.SamEditProductUsage(ctx, id)
		if err != nil {
			return fmt.Errorf("read SAM product usage: %w", err)
		}
		updated = productFromEditRow(db.SamEditProductByIDRow(row), usage)
		return nil
	})
	if err != nil {
		return Product{}, err
	}
	return updated, nil
}

// DeleteProduct removes an unused catalog entry. Entries with licenses,
// installations or aliases must be archived instead.
func (s *Service) DeleteProduct(ctx context.Context, id int64) error {
	actor, err := requireEditor(ctx)
	if err != nil {
		return err
	}
	return s.editRun(ctx, func(edits Store) error {
		before, err := edits.SamEditProductByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("product.not_found")
		}
		if err != nil {
			return fmt.Errorf("read SAM product: %w", err)
		}
		deleted, err := edits.SamDeleteProduct(ctx, id)
		if err != nil {
			return fmt.Errorf("delete SAM product: %w", err)
		}
		if deleted == 0 {
			return ErrConflict
		}
		return auditEdit(ctx, edits, actor, "product.delete", "product", fmt.Sprint(id), before, nil)
	})
}

func licenseParams(input LicenseInput, validUntil pgtype.Date) db.SamEditCreateLicenseParams {
	currency := input.Currency
	if currency == "" {
		currency = "RUB"
	}
	return db.SamEditCreateLicenseParams{
		ProductID:   input.ProductID,
		LicenseType: input.Kind,
		Seats:       int64(input.SeatsTotal),
		CostPerSeat: input.CostPerSeat,
		Currency:    currency,
		ExpiresAt:   validUntil,
		Contract:    input.Contract,
	}
}

// licenseFromView maps the public license shape after a write.
func licenseFromView(row db.SamEditLicenseViewRow) License {
	validUntil := (*string)(nil)
	if row.ValidUntil.Valid {
		day := row.ValidUntil.Time.Format("2006-01-02")
		validUntil = &day
	}
	return License{
		ID:         row.ID,
		Product:    row.ProductName,
		Vendor:     row.ProductVendor,
		Kind:       row.Kind,
		SeatsTotal: row.SeatsTotal,
		SeatsUsed:  row.SeatsUsed,
		Contract:   row.Contract,
		ValidUntil: validUntil,
		Status:     row.Status,
	}
}

// CreateLicense adds a license contract for a catalog product.
func (s *Service) CreateLicense(ctx context.Context, input LicenseInput) (License, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return License{}, err
	}
	if err := validateLicenseKind(input.Kind); err != nil {
		return License{}, err
	}
	validUntil, err := parseDate(input.ValidUntil)
	if err != nil {
		return License{}, err
	}
	var created License
	err = s.editRun(ctx, func(edits Store) error {
		if _, err := edits.SamProductByID(ctx, input.ProductID); errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: product %d not found", ErrValidation, input.ProductID)
		}
		row, err := edits.SamEditCreateLicense(ctx, licenseParams(input, validUntil))
		if err != nil {
			return fmt.Errorf("create SAM license: %w", err)
		}
		if err := auditEdit(ctx, edits, actor, "license.create", "license", fmt.Sprint(row.ID), nil, row); err != nil {
			return err
		}
		view, err := edits.SamEditLicenseView(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("read SAM license view: %w", err)
		}
		created = licenseFromView(view)
		return nil
	})
	if err != nil {
		return License{}, err
	}
	return created, nil
}

// UpdateLicense replaces the editable fields of one license contract.
func (s *Service) UpdateLicense(ctx context.Context, id int64, input LicenseInput) (License, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return License{}, err
	}
	if err := validateLicenseKind(input.Kind); err != nil {
		return License{}, err
	}
	validUntil, err := parseDate(input.ValidUntil)
	if err != nil {
		return License{}, err
	}
	var updated License
	err = s.editRun(ctx, func(edits Store) error {
		before, err := edits.SamEditLicenseByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("license.not_found")
		}
		if err != nil {
			return fmt.Errorf("read SAM license: %w", err)
		}
		row, err := edits.SamEditUpdateLicense(ctx, db.SamEditUpdateLicenseParams{
			ID:          id,
			ProductID:   input.ProductID,
			LicenseType: input.Kind,
			Seats:       int64(input.SeatsTotal),
			CostPerSeat: input.CostPerSeat,
			Currency: func() string {
				if input.Currency == "" {
					return before.Currency
				}
				return input.Currency
			}(),
			StartsAt:  before.StartsAt,
			ExpiresAt: validUntil,
			Contract:  input.Contract,
		})
		if err != nil {
			return fmt.Errorf("update SAM license: %w", err)
		}
		if err := auditEdit(ctx, edits, actor, "license.update", "license", fmt.Sprint(id), before, row); err != nil {
			return err
		}
		view, err := edits.SamEditLicenseView(ctx, id)
		if err != nil {
			return fmt.Errorf("read SAM license view: %w", err)
		}
		updated = licenseFromView(view)
		return nil
	})
	if err != nil {
		return License{}, err
	}
	return updated, nil
}

// DeleteLicense removes one license contract.
func (s *Service) DeleteLicense(ctx context.Context, id int64) error {
	actor, err := requireEditor(ctx)
	if err != nil {
		return err
	}
	return s.editRun(ctx, func(edits Store) error {
		before, err := edits.SamEditLicenseByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("license.not_found")
		}
		if err != nil {
			return fmt.Errorf("read SAM license: %w", err)
		}
		deleted, err := edits.SamEditDeleteLicense(ctx, id)
		if err != nil {
			return fmt.Errorf("delete SAM license: %w", err)
		}
		if deleted == 0 {
			return huma.Error404NotFound("license.not_found")
		}
		return auditEdit(ctx, edits, actor, "license.delete", "license", fmt.Sprint(id), before, nil)
	})
}

func installationParams(input InstallationInput, productID pgtype.Int8) db.SamEditCreateInstallationParams {
	return db.SamEditCreateInstallationParams{
		RawName:    input.RawName,
		DeviceName: input.DeviceName,
		UserName:   input.UserName,
		Version:    input.Version,
		ProductID:  productID,
		Status: func() string {
			if input.Status == "" {
				return "installed"
			}
			return input.Status
		}(),
	}
}

func installationFromView(row db.SamEditInstallationViewRow) Installation {
	var productID *int64
	if row.ProductID.Valid {
		value := row.ProductID.Int64
		productID = &value
	}
	product := ""
	if row.ProductName.Valid {
		product = row.ProductName.String
	}
	return Installation{
		ID:         row.ID,
		ProductID:  productID,
		Product:    product,
		RawName:    row.RawName,
		DeviceName: row.DeviceName,
		UserName:   row.UserName,
		Version:    row.Version,
		Source:     row.Source,
		Status:     row.Status,
		LastSeenAt: row.LastSeenAt.Time,
	}
}

func optionalProductID(raw *int64) pgtype.Int8 {
	if raw == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *raw, Valid: true}
}

// CreateInstallation records a manual installation row.
func (s *Service) CreateInstallation(ctx context.Context, input InstallationInput) (Installation, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return Installation{}, err
	}
	if err := validateInstallationStatus(input.Status); err != nil {
		return Installation{}, err
	}
	productID := optionalProductID(input.ProductID)
	var created Installation
	err = s.editRun(ctx, func(edits Store) error {
		row, err := edits.SamEditCreateInstallation(ctx, installationParams(input, productID))
		if err != nil {
			return fmt.Errorf("create SAM installation: %w", err)
		}
		if err := auditEdit(ctx, edits, actor, "installation.create", "installation", fmt.Sprint(row.ID), nil, row); err != nil {
			return err
		}
		view, err := edits.SamEditInstallationView(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("read SAM installation view: %w", err)
		}
		created = installationFromView(view)
		return nil
	})
	if err != nil {
		return Installation{}, err
	}
	return created, nil
}

// UpdateInstallation edits one installation row, including re-linking it to
// another catalog product.
func (s *Service) UpdateInstallation(ctx context.Context, id int64, input InstallationInput) (Installation, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return Installation{}, err
	}
	if err := validateInstallationStatus(input.Status); err != nil {
		return Installation{}, err
	}
	productID := optionalProductID(input.ProductID)
	var updated Installation
	err = s.editRun(ctx, func(edits Store) error {
		before, err := edits.SamEditInstallationByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("installation.not_found")
		}
		if err != nil {
			return fmt.Errorf("read SAM installation: %w", err)
		}
		row, err := edits.SamEditUpdateInstallation(ctx, db.SamEditUpdateInstallationParams{
			ID:         id,
			RawName:    input.RawName,
			DeviceName: input.DeviceName,
			UserName:   input.UserName,
			Version:    input.Version,
			ProductID:  productID,
			Status:     input.Status,
		})
		if err != nil {
			return fmt.Errorf("update SAM installation: %w", err)
		}
		if err := auditEdit(ctx, edits, actor, "installation.update", "installation", fmt.Sprint(id), before, row); err != nil {
			return err
		}
		view, err := edits.SamEditInstallationView(ctx, id)
		if err != nil {
			return fmt.Errorf("read SAM installation view: %w", err)
		}
		updated = installationFromView(view)
		return nil
	})
	if err != nil {
		return Installation{}, err
	}
	return updated, nil
}

// DeleteInstallation removes one installation row.
func (s *Service) DeleteInstallation(ctx context.Context, id int64) error {
	actor, err := requireEditor(ctx)
	if err != nil {
		return err
	}
	return s.editRun(ctx, func(edits Store) error {
		before, err := edits.SamEditInstallationByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("installation.not_found")
		}
		if err != nil {
			return fmt.Errorf("read SAM installation: %w", err)
		}
		deleted, err := edits.SamEditDeleteInstallation(ctx, id)
		if err != nil {
			return fmt.Errorf("delete SAM installation: %w", err)
		}
		if deleted == 0 {
			return huma.Error404NotFound("installation.not_found")
		}
		return auditEdit(ctx, edits, actor, "installation.delete", "installation", fmt.Sprint(id), before, nil)
	})
}

// ProductAliases lists the known aliases of one product.
func (s *Service) ProductAliases(ctx context.Context, productID int64) ([]Alias, error) {
	if _, err := requireEditor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.store.SamEditAliasesByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list SAM aliases: %w", err)
	}
	aliases := make([]Alias, 0, len(rows))
	for _, row := range rows {
		aliases = append(aliases, Alias{ID: row.ID, Alias: row.Alias})
	}
	return aliases, nil
}

// AddAlias records a known alias for a product; matching uses normalized
// aliases to clean dirty process names. Returns the alias rows of the product.
func (s *Service) AddAlias(ctx context.Context, productID int64, input AliasInput) ([]Alias, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return nil, err
	}
	normalized := importer.NormalizeName(input.Alias)
	if normalized == "" {
		return nil, fmt.Errorf("%w: alias", ErrValidation)
	}
	var aliases []Alias
	err = s.editRun(ctx, func(edits Store) error {
		if _, err := edits.SamProductByID(ctx, productID); errors.Is(err, pgx.ErrNoRows) {
			return huma.Error404NotFound("product.not_found")
		}
		rows, err := edits.SamCreateAliases(ctx, db.SamCreateAliasesParams{
			ProductID:         productID,
			Aliases:           []string{input.Alias},
			NormalizedAliases: []string{normalized},
			CreatedBy:         actor,
		})
		if err != nil {
			return fmt.Errorf("create SAM alias: %w", err)
		}
		if len(rows) == 0 {
			return ErrConflict
		}
		if err := auditEdit(ctx, edits, actor, "alias.create", "product", fmt.Sprint(productID), nil, rows); err != nil {
			return err
		}
		aliases = make([]Alias, 0, len(rows))
		for _, row := range rows {
			aliases = append(aliases, Alias{ID: row.ID, Alias: row.Alias})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return aliases, nil
}

// DeleteAlias removes one alias row.
func (s *Service) DeleteAlias(ctx context.Context, aliasID int64) error {
	actor, err := requireEditor(ctx)
	if err != nil {
		return err
	}
	return s.editRun(ctx, func(edits Store) error {
		deleted, err := edits.SamDeleteAliases(ctx, []int64{aliasID})
		if err != nil {
			return fmt.Errorf("delete SAM alias: %w", err)
		}
		if deleted == 0 {
			return huma.Error404NotFound("alias.not_found")
		}
		return auditEdit(ctx, edits, actor, "alias.delete", "alias", fmt.Sprint(aliasID), nil, nil)
	})
}
