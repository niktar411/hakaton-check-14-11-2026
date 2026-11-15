package localaccounts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
)

const upsertInitialAdmin = `
INSERT INTO local_accounts(username, display_name, password_hash, role, color, enabled)
VALUES ($1, $1, $2, 'admin', $3, true)
ON CONFLICT(username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    role = 'admin',
    color = EXCLUDED.color,
    enabled = true,
    updated_at = now()
RETURNING id, username, display_name, password_hash, role, enabled, color,
          personal_context, tone, created_at, updated_at`

// BootstrapDatabase is the small pgx surface needed by BootstrapAdmin.
type BootstrapDatabase interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// BootstrapAdmin creates the configured administrator or promotes and updates
// an existing account with the same username. Profile text and display name are
// preserved on updates. The returned API model never contains the hash.
func BootstrapAdmin(ctx context.Context, database BootstrapDatabase, username, password, color string) (LocalAccount, error) {
	if err := validateUsername(username); err != nil {
		return LocalAccount{}, fmt.Errorf("bootstrap administrator username: %w", err)
	}
	if err := validatePassword(password); err != nil {
		return LocalAccount{}, fmt.Errorf("bootstrap administrator password: %w", err)
	}
	if !colorPattern.MatchString(color) {
		return LocalAccount{}, fmt.Errorf("bootstrap administrator color: %w", ErrValidation)
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return LocalAccount{}, fmt.Errorf("hash bootstrap administrator password: %w", err)
	}

	var account db.LocalAccount
	if err := database.QueryRow(ctx, upsertInitialAdmin, username, passwordHash, color).Scan(
		&account.ID, &account.Username, &account.DisplayName, &account.PasswordHash,
		&account.Role, &account.Enabled, &account.Color, &account.PersonalContext,
		&account.Tone, &account.CreatedAt, &account.UpdatedAt,
	); err != nil {
		return LocalAccount{}, fmt.Errorf("upsert bootstrap administrator: %w", err)
	}
	return localAccountFromDB(account), nil
}
