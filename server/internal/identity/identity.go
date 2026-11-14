package identity

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	db "hackathon/internal/db/sqlc"
)

type contextKey struct{}
type requestIPKey struct{}

type Principal struct {
	Session  db.Session
	Account  db.LocalAccount
	AuthKind string
	IP       string
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func WithRequestIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, requestIPKey{}, ip)
}

func RequestIP(ctx context.Context) string {
	ip, _ := ctx.Value(requestIPKey{}).(string)
	return ip
}

func FromContext(ctx context.Context) (Principal, error) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	if !ok {
		return Principal{}, huma.Error401Unauthorized("auth.required")
	}
	return principal, nil
}

func IsAdmin(principal Principal, adminUser, adminIP string) bool {
	return principal.AuthKind == "local" && principal.Account.Enabled &&
		principal.Account.Role == "admin" && principal.Account.Username == adminUser &&
		principal.Session.Username == adminUser && principal.IP == adminIP
}

func RequireAdmin(ctx context.Context, adminUser, adminIP string) (Principal, error) {
	principal, err := FromContext(ctx)
	if err != nil {
		return Principal{}, err
	}
	if !IsAdmin(principal, adminUser, adminIP) {
		return Principal{}, huma.Error403Forbidden("auth.admin_vpn_required")
	}
	return principal, nil
}
