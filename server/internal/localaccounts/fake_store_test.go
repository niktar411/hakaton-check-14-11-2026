package localaccounts

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

type fakeStore struct {
	mu            sync.Mutex
	accounts      map[int64]db.LocalAccount
	sessions      map[string]db.CreateSessionParams
	conversations map[int64]db.Conversation
	messages      map[int64][]db.Message
	errors        map[string]error
	calls         []string
	nextID        int64
}

func newFakeStore(accounts ...db.LocalAccount) *fakeStore {
	store := &fakeStore{
		accounts: make(map[int64]db.LocalAccount), sessions: make(map[string]db.CreateSessionParams),
		conversations: make(map[int64]db.Conversation), messages: make(map[int64][]db.Message),
		errors: make(map[string]error), nextID: 1,
	}
	for _, account := range accounts {
		store.accounts[account.ID] = account
		if account.ID >= store.nextID {
			store.nextID = account.ID + 1
		}
	}
	return store
}

func (f *fakeStore) beforeCall(ctx context.Context, name string) error {
	f.calls = append(f.calls, name)
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.errors[name]
}

func (f *fakeStore) CreateLocalAccount(ctx context.Context, arg db.CreateLocalAccountParams) (db.LocalAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "create"); err != nil {
		return db.LocalAccount{}, err
	}
	for _, account := range f.accounts {
		if account.Username == arg.Username {
			return db.LocalAccount{}, &pgconn.PgError{Code: "23505"}
		}
	}
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	account := db.LocalAccount{
		ID: f.nextID, Username: arg.Username, DisplayName: arg.DisplayName,
		PasswordHash: arg.PasswordHash, Role: arg.Role, Enabled: true, Color: arg.Color,
		PersonalContext: arg.PersonalContext, Tone: arg.Tone, CreatedAt: now, UpdatedAt: now,
	}
	f.nextID++
	f.accounts[account.ID] = account
	return account, nil
}

func (f *fakeStore) GetLocalAccountByUsername(ctx context.Context, username string) (db.LocalAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "get_by_username"); err != nil {
		return db.LocalAccount{}, err
	}
	for _, account := range f.accounts {
		if account.Username == username {
			return account, nil
		}
	}
	return db.LocalAccount{}, pgx.ErrNoRows
}

func (f *fakeStore) GetLocalAccountByID(ctx context.Context, id int64) (db.LocalAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "get_by_id"); err != nil {
		return db.LocalAccount{}, err
	}
	account, ok := f.accounts[id]
	if !ok {
		return db.LocalAccount{}, pgx.ErrNoRows
	}
	return account, nil
}

func (f *fakeStore) ListLocalAccounts(ctx context.Context, arg db.ListLocalAccountsParams) ([]db.LocalAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "list"); err != nil {
		return nil, err
	}
	accounts := make([]db.LocalAccount, 0, len(f.accounts))
	for _, account := range f.accounts {
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].Username < accounts[j].Username })
	start := min(int(arg.Offset), len(accounts))
	end := min(start+int(arg.Limit), len(accounts))
	return accounts[start:end], nil
}

func (f *fakeStore) UpdateLocalAccountProfile(ctx context.Context, arg db.UpdateLocalAccountProfileParams) (db.LocalAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "profile"); err != nil {
		return db.LocalAccount{}, err
	}
	account, ok := f.accounts[arg.ID]
	if !ok {
		return db.LocalAccount{}, pgx.ErrNoRows
	}
	account.DisplayName = arg.DisplayName
	account.Color = arg.Color
	account.PersonalContext = arg.PersonalContext
	account.Tone = arg.Tone
	account.UpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	f.accounts[arg.ID] = account
	return account, nil
}

func (f *fakeStore) SetLocalAccountEnabled(ctx context.Context, arg db.SetLocalAccountEnabledParams) (db.LocalAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "enabled"); err != nil {
		return db.LocalAccount{}, err
	}
	account, ok := f.accounts[arg.ID]
	if !ok {
		return db.LocalAccount{}, pgx.ErrNoRows
	}
	account.Enabled = arg.Enabled
	account.UpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	f.accounts[arg.ID] = account
	return account, nil
}

func (f *fakeStore) SetLocalAccountPassword(ctx context.Context, arg db.SetLocalAccountPasswordParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "password"); err != nil {
		return err
	}
	account, ok := f.accounts[arg.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	account.PasswordHash = arg.PasswordHash
	account.UpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	f.accounts[arg.ID] = account
	return nil
}

func (f *fakeStore) DeleteSessionsByAccount(ctx context.Context, accountID pgtype.Int8) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "delete_sessions"); err != nil {
		return err
	}
	for id, session := range f.sessions {
		if session.AccountID.Valid && session.AccountID.Int64 == accountID.Int64 {
			delete(f.sessions, id)
		}
	}
	return nil
}

func (f *fakeStore) ListAccountConversations(ctx context.Context, arg db.ListAccountConversationsParams) ([]db.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "list_conversations"); err != nil {
		return nil, err
	}
	conversations := make([]db.Conversation, 0)
	for _, conversation := range f.conversations {
		if conversation.Username == arg.Username {
			conversations = append(conversations, conversation)
		}
	}
	sort.Slice(conversations, func(i, j int) bool { return conversations[i].ID > conversations[j].ID })
	start := min(int(arg.Offset), len(conversations))
	end := min(start+int(arg.Limit), len(conversations))
	return conversations[start:end], nil
}

func (f *fakeStore) GetConversationForAdmin(ctx context.Context, id int64) (db.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "get_conversation"); err != nil {
		return db.Conversation{}, err
	}
	conversation, ok := f.conversations[id]
	if !ok {
		return db.Conversation{}, pgx.ErrNoRows
	}
	return conversation, nil
}

func (f *fakeStore) CreateSession(ctx context.Context, arg db.CreateSessionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "create_session"); err != nil {
		return err
	}
	if _, exists := f.sessions[arg.ID]; exists {
		return &pgconn.PgError{Code: "23505"}
	}
	f.sessions[arg.ID] = arg
	return nil
}

func (f *fakeStore) ListMessages(ctx context.Context, conversationID int64) ([]db.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.beforeCall(ctx, "messages"); err != nil {
		return nil, err
	}
	messages := append([]db.Message(nil), f.messages[conversationID]...)
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID < messages[j].ID })
	if len(messages) > 80 {
		messages = messages[:80]
	}
	return messages, nil
}

func (f *fakeStore) callCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, call := range f.calls {
		if call == name {
			count++
		}
	}
	return count
}
