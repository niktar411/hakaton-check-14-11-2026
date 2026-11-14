package operations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
)

type principal = identity.Principal

func opaque() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(s string) string                   { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func timestamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	ip := net.ParseIP(host)
	// The API is only published through the internal nginx container on the
	// dedicated Docker bridge. That container is the reverse proxy, so the
	// original client address from X-Forwarded-For is authoritative there.
	// Direct clients (for example a VPN peer) cannot use the header.
	if ip != nil && (ip.IsLoopback() || internalProxyNetwork.Contains(ip)) {
		// nginx overwrites X-Real-IP with the real peer address. Falling back
		// to the last X-Forwarded-For entry avoids trusting a client-supplied
		// prefix of the chain.
		forwarded := strings.TrimSpace(r.Header.Get("X-Real-IP"))
		if forwarded == "" {
			chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
			forwarded = strings.TrimSpace(chain[len(chain)-1])
		}
		if forwardedIP := net.ParseIP(forwarded); forwardedIP != nil {
			return forwardedIP.String()
		}
	}
	return host
}

var internalProxyNetwork = func() *net.IPNet {
	_, network, err := net.ParseCIDR("172.30.0.0/24")
	if err != nil {
		panic("invalid internal proxy network")
	}
	return network
}()
func allowedUser(u string) bool {
	switch u {
	case "niktar411", "reznak", "kvant", "hensy":
		return true
	}
	return false
}

func (s *Service) Identity(ctx context.Context) (Identity, error) {
	p, err := identity.FromContext(ctx)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Username: p.Session.Username, Admin: identity.IsAdmin(p, s.config.AdminUser, s.config.AdminIP), CSRF: p.Session.CsrfToken, ModelReady: s.model.Ready(ctx)}, nil
}
func (s *Service) admin(ctx context.Context) (principal, error) {
	return identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP)
}

func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie("portal_session")
		if err != nil {
			http.Error(w, "auth.required", 401)
			return
		}
		session, err := s.db.GetSession(r.Context(), digest(cookie.Value))
		if err != nil {
			http.Error(w, "auth.required", 401)
			return
		}
		p := principal{Session: session, IP: remoteIP(r), AuthKind: "oauth"}
		if session.AccountID.Valid {
			account, accountErr := s.db.GetLocalAccountByID(r.Context(), session.AccountID.Int64)
			if accountErr != nil || !account.Enabled || account.Username != session.Username {
				http.Error(w, "auth.session_invalid", 401)
				return
			}
			p.Account = account
			if !session.AccessToken.Valid || session.AccessToken.String == "" {
				p.AuthKind = "local"
			}
		}
		if p.AuthKind == "oauth" {
			username, verifyErr := s.gitLabUser(r.Context(), session.AccessToken.String)
			if verifyErr != nil || username != session.Username || !allowedUser(username) {
				http.Error(w, "auth.session_invalid", 401)
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if origin != s.config.PublicURL && origin != s.config.PlannerURL &&
				(s.config.AIURL == "" || origin != s.config.AIURL) {
				http.Error(w, "auth.csrf", 403)
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CsrfToken)) != 1 {
				http.Error(w, "auth.csrf", 403)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(identity.WithPrincipal(r.Context(), p)))
	})
}

func (s *Service) gitLabUser(ctx context.Context, token string) (string, error) {
	userURL := s.config.GitLabURL + "/api/v4/user"
	if s.config.Provider == "gitea" {
		userURL = s.config.GitLabURL + "/api/v1/user"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", userURL, nil)
	if err != nil {
		return "", fmt.Errorf("create identity request: %w", err)
	}
	if s.config.Provider == "gitea" {
		req.Header.Set("Authorization", "token "+token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("verify identity: %w", err)
	}
	defer closeBody(ctx, res.Body)
	if res.StatusCode != 200 {
		return "", fmt.Errorf("identity rejected: %d", res.StatusCode)
	}
	var result struct {
		Username string `json:"username"`
		Login    string `json:"login"`
		State    string `json:"state"`
		Active   bool   `json:"active"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&result); err != nil {
		return "", fmt.Errorf("decode identity: %w", err)
	}
	if s.config.Provider == "gitea" {
		if !result.Active {
			return "", fmt.Errorf("identity inactive")
		}
		if result.Login != "" {
			return result.Login, nil
		}
		return result.Username, nil
	}
	if result.State != "active" {
		return "", fmt.Errorf("identity inactive")
	}
	return result.Username, nil
}
func closeBody(ctx context.Context, b io.Closer) {
	if err := b.Close(); err != nil {
		slog.WarnContext(ctx, "close response failed")
	}
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	state, verifier := opaque(), opaque()
	if state == "" || verifier == "" {
		http.Error(w, "auth.unavailable", 503)
		return
	}
	if err := s.db.CreateOAuthState(r.Context(), db.CreateOAuthStateParams{ID: digest(state), Verifier: verifier, ExpiresAt: timestamp(time.Now().Add(10 * time.Minute))}); err != nil {
		http.Error(w, "auth.unavailable", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_oauth", Value: state, Path: "/auth", HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	sum := sha256.Sum256([]byte(verifier))
	scope := "read_user"
	authorizeURL := s.config.GitLabURL + "/oauth/authorize"
	if s.config.Provider == "gitea" {
		scope = "user:read"
		authorizeURL = s.config.GitLabURL + "/login/oauth/authorize"
	}
	q := url.Values{"client_id": {s.config.OAuthID}, "redirect_uri": {s.config.PublicURL + "/auth/callback"}, "response_type": {"code"}, "scope": {scope}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}}
	http.Redirect(w, r, authorizeURL+"?"+q.Encode(), http.StatusFound)
}

func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("portal_oauth")
	state := r.URL.Query().Get("state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		http.Error(w, "auth.state_invalid", 403)
		return
	}
	verifier, err := s.db.ConsumeOAuthState(r.Context(), digest(state))
	if err != nil {
		http.Error(w, "auth.state_expired", 403)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_oauth", Path: "/auth", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	q := url.Values{"client_id": {s.config.OAuthID}, "client_secret": {s.config.OAuthSecret}, "code": {r.URL.Query().Get("code")}, "grant_type": {"authorization_code"}, "redirect_uri": {s.config.PublicURL + "/auth/callback"}, "code_verifier": {verifier}}
	tokenURL := s.config.GitLabURL + "/oauth/token"
	if s.config.Provider == "gitea" {
		tokenURL = s.config.GitLabURL + "/login/oauth/access_token"
	}
	req, err := http.NewRequestWithContext(r.Context(), "POST", tokenURL, strings.NewReader(q.Encode()))
	if err != nil {
		http.Error(w, "auth.unavailable", 503)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := s.http.Do(req)
	if err != nil {
		http.Error(w, "auth.unavailable", 503)
		return
	}
	defer closeBody(r.Context(), res.Body)
	if res.StatusCode != 200 {
		http.Error(w, "auth.exchange_failed", 403)
		return
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&token); err != nil || token.AccessToken == "" || token.ExpiresIn <= 0 {
		http.Error(w, "auth.exchange_failed", 403)
		return
	}
	username, err := s.gitLabUser(r.Context(), token.AccessToken)
	if err != nil || !allowedUser(username) {
		http.Error(w, "auth.not_in_team", 403)
		return
	}
	color := "#2a78ff"
	switch username {
	case "reznak":
		color = "#0f9b8e"
	case "kvant":
		color = "#ff9300"
	case "hensy":
		color = "#16a34a"
	}
	account, err := s.db.UpsertOAuthAccount(r.Context(), db.UpsertOAuthAccountParams{Username: username, Color: color})
	if err != nil {
		http.Error(w, "auth.unavailable", 503)
		return
	}
	id, csrf := opaque(), opaque()
	if id == "" || csrf == "" {
		http.Error(w, "auth.unavailable", 503)
		return
	}
	lifetime := min(token.ExpiresIn, 7200)
	err = s.db.CreateSession(r.Context(), db.CreateSessionParams{ID: digest(id), Username: username, AccessToken: pgtype.Text{String: token.AccessToken, Valid: true}, CsrfToken: csrf, ExpiresAt: timestamp(time.Now().Add(time.Duration(lifetime) * time.Second)), AccountID: pgtype.Int8{Int64: account.ID, Valid: true}})
	if err != nil {
		http.Error(w, "auth.unavailable", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: id, Path: "/", MaxAge: int(lifetime), HttpOnly: true, Secure: strings.HasPrefix(s.config.PublicURL, "https:"), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusFound)
}
type logoutOutput struct {
	SetCookie string `header:"Set-Cookie"`
}

func (s *Service) Logout(ctx context.Context) (*logoutOutput, error) {
	p, err := identity.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteSession(ctx, p.Session.ID); err != nil {
		return nil, huma.Error503ServiceUnavailable("auth.unavailable")
	}
	secure := ""
	if strings.HasPrefix(s.config.PublicURL, "https:") {
		secure = "; Secure"
	}
	return &logoutOutput{
		SetCookie: "portal_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax" + secure,
	}, nil
}
