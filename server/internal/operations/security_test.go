package operations

import (
	"net/http/httptest"
	"testing"

	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
)

func TestAdminRequiresLocalAdminAccountAndActualVPNAddress(t *testing.T) {
	c := Config{AdminUser: "niktar411", AdminIP: "10.77.0.2"}
	for _, tc := range []struct {
		name, user, ip, kind, role string
		enabled, allowed           bool
	}{
		{name: "local admin VPN", user: "niktar411", ip: "10.77.0.2", kind: "local", role: "admin", enabled: true, allowed: true},
		{name: "wrong IP", user: "niktar411", ip: "10.77.0.3", kind: "local", role: "admin", enabled: true},
		{name: "member", user: "niktar411", ip: "10.77.0.2", kind: "local", role: "member", enabled: true},
		{name: "disabled", user: "niktar411", ip: "10.77.0.2", kind: "local", role: "admin"},
		{name: "OAuth is not admin login", user: "niktar411", ip: "10.77.0.2", kind: "oauth", role: "admin", enabled: true},
		{name: "wrong user", user: "hensy", ip: "10.77.0.2", kind: "local", role: "admin", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{config: c}
			principal := identity.Principal{
				Session: db.Session{Username: tc.user}, IP: tc.ip, AuthKind: tc.kind,
				Account: db.LocalAccount{Username: tc.user, Role: tc.role, Enabled: tc.enabled},
			}
			_, err := s.admin(identity.WithPrincipal(t.Context(), principal))
			if (err == nil) != tc.allowed {
				t.Errorf("allowed=%v, want %v (error %v)", err == nil, tc.allowed, err)
			}
		})
	}
}

func TestForwardedHeaderCannotGrantAdminAccess(t *testing.T) {
	r := httptest.NewRequest("GET", "http://portal/api/admin/dashboard", nil)
	r.RemoteAddr = "10.77.0.3:50000"
	r.Header.Set("X-Forwarded-For", "10.77.0.2")
	r.Header.Set("X-Real-IP", "10.77.0.2")
	if got := remoteIP(r); got != "10.77.0.3" {
		t.Fatalf("trusted spoofed header: %s", got)
	}
}

func TestOnlyLoopbackProxyCanSupplyForwardedAddress(t *testing.T) {
	r := httptest.NewRequest("GET", "http://portal/api/admin/dashboard", nil)
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("X-Forwarded-For", "10.77.0.2")
	if got := remoteIP(r); got != "10.77.0.2" {
		t.Fatalf("trusted nginx address not preserved: %s", got)
	}
	r.Header.Set("X-Forwarded-For", "invalid")
	if got := remoteIP(r); got != "127.0.0.1" {
		t.Fatalf("invalid forwarded address accepted: %s", got)
	}
}

func TestUnknownUsageIsNotZero(t *testing.T) {
	if tokenValue(nil).Valid {
		t.Fatal("missing usage treated as zero")
	}
	bad := int64(-1)
	if tokenValue(&bad).Valid {
		t.Fatal("negative usage accepted")
	}
	zero := int64(0)
	if !tokenValue(&zero).Valid {
		t.Fatal("actual zero rejected")
	}
}

func TestUnauthenticatedAdminRejectedBeforeDatabase(t *testing.T) {
	s := &Service{}
	if _, err := s.Dashboard(t.Context()); err == nil {
		t.Fatal("unauthenticated dashboard accepted")
	}
	if _, err := s.ChangePeer(t.Context(), "egor", "rotate"); err == nil {
		t.Fatal("unauthenticated mutation accepted")
	}
}
