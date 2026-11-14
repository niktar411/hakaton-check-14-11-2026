package localaccounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
)

var (
	ErrNotFound           = errors.New("local-account: not found")
	ErrConflict           = errors.New("local-account: username already exists")
	ErrValidation         = errors.New("local-account: invalid input")
	ErrStorage            = errors.New("local-account: storage unavailable")
	ErrInvalidOrigin      = errors.New("local-account: invalid origin")
	ErrInvalidCredentials = errors.New("local-account: invalid credentials")
	ErrAdminAccess        = errors.New("local-account: administrator access required")
	ErrLoginThrottled     = errors.New("local-account: too many failed logins")
	ErrCannotDisableAdmin = errors.New("local-account: cannot disable current administrator")
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)
	colorPattern    = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// Store is implemented by the generated sqlc queries. Keeping it small makes
// authorization and login behavior testable without a database.
type Store interface {
	CreateLocalAccount(ctx context.Context, arg db.CreateLocalAccountParams) (db.LocalAccount, error)
	GetLocalAccountByUsername(ctx context.Context, username string) (db.LocalAccount, error)
	GetLocalAccountByID(ctx context.Context, id int64) (db.LocalAccount, error)
	ListLocalAccounts(ctx context.Context, arg db.ListLocalAccountsParams) ([]db.LocalAccount, error)
	UpdateLocalAccountProfile(ctx context.Context, arg db.UpdateLocalAccountProfileParams) (db.LocalAccount, error)
	SetLocalAccountEnabled(ctx context.Context, arg db.SetLocalAccountEnabledParams) (db.LocalAccount, error)
	SetLocalAccountPassword(ctx context.Context, arg db.SetLocalAccountPasswordParams) error
	DeleteSessionsByAccount(ctx context.Context, accountID pgtype.Int8) error
	ListAccountConversations(ctx context.Context, arg db.ListAccountConversationsParams) ([]db.Conversation, error)
	GetConversationForAdmin(ctx context.Context, id int64) (db.Conversation, error)
	CreateSession(ctx context.Context, arg db.CreateSessionParams) error
	ListMessages(ctx context.Context, conversationID int64) ([]db.Message, error)
}

// Service implements local authentication and administrator account management.
type Service struct {
	store        Store
	config       Config
	now          func() time.Time
	random       io.Reader
	hashPassword func(string) (string, error)
	dummyHash    string

	throttleMu sync.Mutex
	failures   map[string]loginFailures
}

func NewService(pool *pgxpool.Pool, config Config) *Service {
	return newService(db.New(pool), config)
}

func newService(store Store, config Config) *Service {
	config.PublicURL = strings.TrimRight(config.PublicURL, "/")
	config.PlannerURL = strings.TrimRight(config.PlannerURL, "/")
	return &Service{
		store:        store,
		config:       config,
		now:          time.Now,
		random:       rand.Reader,
		hashPassword: HashPassword,
		dummyHash:    newDummyHash(),
		failures:     make(map[string]loginFailures),
	}
}

// newDummyHash builds an unguessable hash used only to spend the same Argon2
// time when a login names an account that does not exist.
func newDummyHash() string {
	secret := make([]byte, 32)
	salt := make([]byte, argonSaltLength)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return ""
	}
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return ""
	}
	return derivePasswordHash(base64.RawURLEncoding.EncodeToString(secret), salt)
}

func (s *Service) admin(ctx context.Context) (identity.Principal, error) {
	return identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP)
}

// Login verifies a local password and creates an opaque, eight-hour session.
// The returned cookie is the only place where the raw session token is exposed.
func (s *Service) Login(ctx context.Context, request LoginRequest, origin string) (LoginResult, *http.Cookie, error) {
	if !s.validOrigin(origin) {
		return LoginResult{}, nil, ErrInvalidOrigin
	}
	if err := validateLoginRequest(request); err != nil {
		return LoginResult{}, nil, err
	}

	requestIP := identity.RequestIP(ctx)
	throttleKey := requestIP + "\x00" + request.Username
	if !s.loginAllowed(throttleKey) {
		return LoginResult{}, nil, ErrLoginThrottled
	}

	account, err := s.store.GetLocalAccountByUsername(ctx, request.Username)
	if err != nil {
		if isNotFound(err) {
			verifyPassword(s.dummyHash, request.Password)
			s.recordLoginFailure(throttleKey)
			return LoginResult{}, nil, ErrInvalidCredentials
		}
		return LoginResult{}, nil, storeError("get local account by username", err)
	}

	hasLocalPassword := account.PasswordHash != ""
	hash := account.PasswordHash
	if !hasLocalPassword {
		hash = s.dummyHash
	}
	passwordValid := verifyPassword(hash, request.Password) && hasLocalPassword
	if account.Username != request.Username || !passwordValid || !account.Enabled {
		s.recordLoginFailure(throttleKey)
		return LoginResult{}, nil, ErrInvalidCredentials
	}

	if request.Audience == "admin" && (requestIP == "" || s.config.AdminUser == "" || s.config.AdminIP == "" ||
		account.Role != "admin" || account.Username != s.config.AdminUser || requestIP != s.config.AdminIP) {
		return LoginResult{}, nil, ErrAdminAccess
	}

	rawSession, err := s.opaqueToken()
	if err != nil {
		return LoginResult{}, nil, fmt.Errorf("create session token: %w", err)
	}
	csrf, err := s.opaqueToken()
	if err != nil {
		return LoginResult{}, nil, fmt.Errorf("create CSRF token: %w", err)
	}
	expiresAt := s.now().UTC().Add(sessionLifetime)
	if err := s.store.CreateSession(ctx, db.CreateSessionParams{
		ID:          sessionDigest(rawSession),
		Username:    account.Username,
		AccessToken: pgtype.Text{},
		CsrfToken:   csrf,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		AccountID:   pgtype.Int8{Int64: account.ID, Valid: true},
	}); err != nil {
		return LoginResult{}, nil, storeError("create local session", err)
	}
	s.clearLoginFailures(throttleKey)

	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    rawSession,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(sessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   s.config.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}
	return LoginResult{CSRF: csrf, Account: localAccountFromDB(account)}, cookie, nil
}

func (s *Service) validOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	return (s.config.PublicURL != "" && origin == s.config.PublicURL) ||
		(s.config.PlannerURL != "" && origin == s.config.PlannerURL)
}

func (s *Service) opaqueToken() (string, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(s.random, value); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func sessionDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// ListAccounts returns accounts without password hashes.
func (s *Service) ListAccounts(ctx context.Context, limit, offset int32) ([]LocalAccount, error) {
	if _, err := s.admin(ctx); err != nil {
		return nil, err
	}
	limit, offset, err := validatePage(limit, offset)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.ListLocalAccounts(ctx, db.ListLocalAccountsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, storeError("list local accounts", err)
	}
	result := make([]LocalAccount, 0, len(rows))
	for _, row := range rows {
		result = append(result, localAccountFromDB(row))
	}
	return result, nil
}

// CreateAccount creates a local member or administrator.
func (s *Service) CreateAccount(ctx context.Context, request CreateAccountRequest) (LocalAccount, error) {
	if _, err := s.admin(ctx); err != nil {
		return LocalAccount{}, err
	}
	if err := validateCreateAccount(request); err != nil {
		return LocalAccount{}, err
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	passwordHash, err := s.hashPassword(request.Password)
	if err != nil {
		return LocalAccount{}, fmt.Errorf("hash account password: %w", err)
	}
	account, err := s.store.CreateLocalAccount(ctx, db.CreateLocalAccountParams{
		Username:        request.Username,
		DisplayName:     request.DisplayName,
		PasswordHash:    passwordHash,
		Role:            request.Role,
		Color:           request.Color,
		PersonalContext: request.PersonalContext,
		Tone:            request.Tone,
	})
	if err != nil {
		return LocalAccount{}, storeError("create local account", err)
	}
	return localAccountFromDB(account), nil
}

// UpdateProfile updates non-credential account settings.
func (s *Service) UpdateProfile(ctx context.Context, id int64, request ProfileRequest) (LocalAccount, error) {
	if _, err := s.admin(ctx); err != nil {
		return LocalAccount{}, err
	}
	if err := validateID(id); err != nil {
		return LocalAccount{}, err
	}
	if err := validateProfile(request); err != nil {
		return LocalAccount{}, err
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	account, err := s.store.UpdateLocalAccountProfile(ctx, db.UpdateLocalAccountProfileParams{
		ID:              id,
		DisplayName:     request.DisplayName,
		Color:           request.Color,
		PersonalContext: request.PersonalContext,
		Tone:            request.Tone,
	})
	if err != nil {
		return LocalAccount{}, storeError("update local account profile", err)
	}
	return localAccountFromDB(account), nil
}

// SetEnabled changes account availability. Disabling also revokes every session.
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) (LocalAccount, error) {
	principal, err := s.admin(ctx)
	if err != nil {
		return LocalAccount{}, err
	}
	if err := validateID(id); err != nil {
		return LocalAccount{}, err
	}
	current, err := s.store.GetLocalAccountByID(ctx, id)
	if err != nil {
		return LocalAccount{}, storeError("get account before enabled change", err)
	}
	if !enabled && (id == principal.Account.ID || current.Username == s.config.AdminUser) {
		return LocalAccount{}, ErrCannotDisableAdmin
	}
	account, err := s.store.SetLocalAccountEnabled(ctx, db.SetLocalAccountEnabledParams{ID: id, Enabled: enabled})
	if err != nil {
		return LocalAccount{}, storeError("set local account enabled", err)
	}
	if !enabled {
		if err := s.store.DeleteSessionsByAccount(ctx, pgtype.Int8{Int64: id, Valid: true}); err != nil {
			return LocalAccount{}, storeError("delete sessions for disabled account", err)
		}
	}
	return localAccountFromDB(account), nil
}

// ResetSessions revokes all sessions belonging to an account.
func (s *Service) ResetSessions(ctx context.Context, id int64) (LocalAccount, error) {
	if _, err := s.admin(ctx); err != nil {
		return LocalAccount{}, err
	}
	if err := validateID(id); err != nil {
		return LocalAccount{}, err
	}
	account, err := s.store.GetLocalAccountByID(ctx, id)
	if err != nil {
		return LocalAccount{}, storeError("get account before session reset", err)
	}
	if err := s.store.DeleteSessionsByAccount(ctx, pgtype.Int8{Int64: id, Valid: true}); err != nil {
		return LocalAccount{}, storeError("reset local account sessions", err)
	}
	return localAccountFromDB(account), nil
}

// SetPassword replaces a password and revokes existing sessions.
func (s *Service) SetPassword(ctx context.Context, id int64, password string) (LocalAccount, error) {
	if _, err := s.admin(ctx); err != nil {
		return LocalAccount{}, err
	}
	if err := validateID(id); err != nil {
		return LocalAccount{}, err
	}
	if err := validatePassword(password); err != nil {
		return LocalAccount{}, err
	}
	if _, err := s.store.GetLocalAccountByID(ctx, id); err != nil {
		return LocalAccount{}, storeError("get account before password change", err)
	}
	passwordHash, err := s.hashPassword(password)
	if err != nil {
		return LocalAccount{}, fmt.Errorf("hash replacement password: %w", err)
	}
	if err := s.store.SetLocalAccountPassword(ctx, db.SetLocalAccountPasswordParams{ID: id, PasswordHash: passwordHash}); err != nil {
		return LocalAccount{}, storeError("set local account password", err)
	}
	if err := s.store.DeleteSessionsByAccount(ctx, pgtype.Int8{Int64: id, Valid: true}); err != nil {
		return LocalAccount{}, storeError("revoke sessions after password change", err)
	}
	account, err := s.store.GetLocalAccountByID(ctx, id)
	if err != nil {
		return LocalAccount{}, storeError("reload account after password change", err)
	}
	return localAccountFromDB(account), nil
}

// AccountConversations lists an account's conversations in reverse ID order.
func (s *Service) AccountConversations(ctx context.Context, id int64, limit, offset int32) ([]Conversation, error) {
	if _, err := s.admin(ctx); err != nil {
		return nil, err
	}
	if err := validateID(id); err != nil {
		return nil, err
	}
	limit, offset, err := validatePage(limit, offset)
	if err != nil {
		return nil, err
	}
	account, err := s.store.GetLocalAccountByID(ctx, id)
	if err != nil {
		return nil, storeError("get account for conversations", err)
	}
	rows, err := s.store.ListAccountConversations(ctx, db.ListAccountConversationsParams{Username: account.Username, Limit: limit, Offset: offset})
	if err != nil {
		return nil, storeError("list account conversations", err)
	}
	result := make([]Conversation, 0, len(rows))
	for _, row := range rows {
		result = append(result, conversationFromDB(row))
	}
	return result, nil
}

// ConversationForAdmin returns one conversation with its messages.
func (s *Service) ConversationForAdmin(ctx context.Context, id int64) (ConversationDetail, error) {
	if _, err := s.admin(ctx); err != nil {
		return ConversationDetail{}, err
	}
	if err := validateID(id); err != nil {
		return ConversationDetail{}, err
	}
	conversation, err := s.store.GetConversationForAdmin(ctx, id)
	if err != nil {
		return ConversationDetail{}, storeError("get conversation for administrator", err)
	}
	rows, err := s.store.ListMessages(ctx, id)
	if err != nil {
		return ConversationDetail{}, storeError("list conversation messages", err)
	}
	messages := make([]AccountMessage, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, AccountMessage{
			ID: row.ID, ConversationID: row.ConversationID, Role: row.Role,
			Content: row.Content, CreatedAt: row.CreatedAt.Time,
		})
	}
	return ConversationDetail{Conversation: conversationFromDB(conversation), Messages: messages}, nil
}

func localAccountFromDB(account db.LocalAccount) LocalAccount {
	return LocalAccount{
		ID: account.ID, Username: account.Username, DisplayName: account.DisplayName,
		Role: account.Role, Enabled: account.Enabled, Color: account.Color,
		PersonalContext: account.PersonalContext, Tone: account.Tone,
		CreatedAt: account.CreatedAt.Time, UpdatedAt: account.UpdatedAt.Time,
	}
}

func conversationFromDB(conversation db.Conversation) Conversation {
	return Conversation{ID: conversation.ID, Username: conversation.Username, Phase: conversation.Phase, CreatedAt: conversation.CreatedAt.Time}
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}

func storeError(operation string, err error) error {
	if isNotFound(err) {
		return fmt.Errorf("%s: %w", operation, ErrNotFound)
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return fmt.Errorf("%s: %w", operation, ErrConflict)
	}
	return fmt.Errorf("%s: %w", operation, errors.Join(ErrStorage, err))
}

func validateLoginRequest(request LoginRequest) error {
	if err := validateUsername(request.Username); err != nil {
		return err
	}
	if len(request.Password) < 1 || len(request.Password) > 128 || request.Audience != "coding" && request.Audience != "admin" {
		return fmt.Errorf("%w: login request", ErrValidation)
	}
	return nil
}

func validateCreateAccount(request CreateAccountRequest) error {
	if err := validateUsername(request.Username); err != nil {
		return err
	}
	if err := validatePassword(request.Password); err != nil {
		return err
	}
	if request.Role != "member" && request.Role != "admin" {
		return fmt.Errorf("%w: role", ErrValidation)
	}
	return validateProfile(ProfileRequest{
		DisplayName: request.DisplayName, Color: request.Color,
		PersonalContext: request.PersonalContext, Tone: request.Tone,
	})
}

func validateProfile(request ProfileRequest) error {
	if strings.TrimSpace(request.DisplayName) == "" || utf8.RuneCountInString(request.DisplayName) > 100 {
		return fmt.Errorf("%w: display name", ErrValidation)
	}
	if !colorPattern.MatchString(request.Color) {
		return fmt.Errorf("%w: color", ErrValidation)
	}
	if utf8.RuneCountInString(request.PersonalContext) > 16000 || utf8.RuneCountInString(request.Tone) > 2000 {
		return fmt.Errorf("%w: profile text", ErrValidation)
	}
	return nil
}

func validateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("%w: username", ErrValidation)
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 12 || len(password) > 128 {
		return fmt.Errorf("%w: password length", ErrValidation)
	}
	return nil
}

func validateID(id int64) error {
	if id < 1 {
		return fmt.Errorf("%w: id", ErrValidation)
	}
	return nil
}

func validatePage(limit, offset int32) (int32, int32, error) {
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return 0, 0, fmt.Errorf("%w: pagination", ErrValidation)
	}
	return limit, offset, nil
}
