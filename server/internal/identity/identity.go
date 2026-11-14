package identity

import (
	"context"
	"strings"

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

// IsAdmin accepts one or more comma-separated administrator addresses. The
// list lets the same administrator use both the legacy WireGuard profile and
// the current team proxy without weakening the exact-address check.
func IsAdmin(principal Principal, adminUser, adminIP string) bool {
	return principal.AuthKind == "local" && principal.Account.Enabled &&
		principal.Account.Role == "admin" && principal.Account.Username == adminUser &&
		principal.Session.Username == adminUser && AdminAddressAllowed(principal.IP, adminIP)
}

// AdminAddressAllowed reports whether actual is one of the comma-separated
// administrator addresses.
func AdminAddressAllowed(actual, allowed string) bool {
	if actual == "" {
		return false
	}
	for _, candidate := range strings.Split(allowed, ",") {
		if actual == strings.TrimSpace(candidate) {
			return true
		}
	}
	return false
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
