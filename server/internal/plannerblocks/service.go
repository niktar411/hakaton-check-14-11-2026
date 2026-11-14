package plannerblocks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/identity"
	"hackathon/internal/notifications"
)

const (
	publicListLimit          int32 = 200
	maxConcurrentGitLabCalls       = 8
	listCacheTTL                   = 30 * time.Second
	gitLabRequestError             = "gitlab.merge_request_unavailable"
	gitLabApprovalsError           = "gitlab.approvals_unavailable"
	gitLabProgressError            = "gitlab.progress_unavailable"
)

var (
	ErrNotFound         = errors.New("planner block: not found")
	ErrOwnerNotFound    = errors.New("planner block: owner not found")
	ErrPositionConflict = errors.New("planner block: position already exists")
	ErrInvalidInput     = errors.New("planner block: invalid input")
)

// Config controls the single administrator principal allowed to mutate blocks.
type Config struct {
	AdminUser string
	AdminIP   string
}

// GitLab is the subset of the shared GitLab client used by the planner.
type GitLab interface {
	MergeRequestByIID(ctx context.Context, iid int64) (gitlab.MergeRequest, error)
	MergeRequestApprovals(ctx context.Context, iid int64) (gitlab.Approvals, error)
}

// Store is implemented by db.Queries. Keeping the generated methods behind an
// interface makes authorization and progress logic testable without a database.
type Store interface {
	ListPlannerBlocks(ctx context.Context, arg db.ListPlannerBlocksParams) ([]db.ListPlannerBlocksRow, error)
	CreatePlannerBlock(ctx context.Context, arg db.CreatePlannerBlockParams) (db.PlannerBlock, error)
	NextPlannerPosition(ctx context.Context) (int32, error)
	GetPlannerBlock(ctx context.Context, id int64) (db.PlannerBlock, error)
	UpdatePlannerBlock(ctx context.Context, arg db.UpdatePlannerBlockParams) (db.PlannerBlock, error)
	DeletePlannerBlock(ctx context.Context, id int64) error
	SetPlannerBlockMergeRequest(ctx context.Context, arg db.SetPlannerBlockMergeRequestParams) error
	StartPlannerBlock(ctx context.Context, id int64) (db.PlannerBlock, error)
	SetPlannerBlockCompleted(ctx context.Context, id int64) (int64, error)
	ListChatPromptRecipients(ctx context.Context, owner string) ([]string, error)
	CreateChatPromptIfAbsent(ctx context.Context, arg db.CreateChatPromptIfAbsentParams) error
	GetLocalAccountByID(ctx context.Context, id int64) (db.LocalAccount, error)
}

// Service owns planner persistence, administrator authorization and live
// GitLab progress lookup. Mutations are serialized so next-position allocation
// cannot race within the service; the database unique index remains authoritative.
type Service struct {
	store  Store
	gitLab GitLab
	config Config
	mu     sync.Mutex
	// notifier is optional: when set, a block whose merge request is observed
	// as merged for the first time produces a chat notification.
	notifier notifications.Notifier
	// listCache memoizes the most recent GitLab-backed snapshot. The client
	// polls every 15 seconds; without the cache each poll re-runs dozens of
	// GitLab lookups and the next poll aborts them mid-flight (context
	// canceled), leaving completion transitions half-done. Mutations
	// invalidate the cache so state changes are never served stale.
	listCache   []PlannerBlock
	listCacheAt time.Time
}

// SetNotifier attaches a chat notification sink. Safe to call before serving;
// the planner passes nil checks at the call sites.
func (s *Service) SetNotifier(notifier notifications.Notifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifier = notifier
}

func NewService(pool *pgxpool.Pool, gitLab GitLab, config Config) *Service {
	return newService(db.New(pool), gitLab, config)
}

func newService(store Store, gitLab GitLab, config Config) *Service {
	return &Service{store: store, gitLab: gitLab, config: config}
}

// List returns a bounded team snapshot. Failure of one GitLab lookup affects
// only that block and never replaces a known state with fabricated progress.
func (s *Service) List(ctx context.Context) ([]PlannerBlock, error) {
	s.mu.Lock()
	if s.listCache != nil && time.Since(s.listCacheAt) < listCacheTTL {
		out := make([]PlannerBlock, len(s.listCache))
		copy(out, s.listCache)
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()

	rows, err := s.store.ListPlannerBlocks(ctx, db.ListPlannerBlocksParams{Limit: publicListLimit})
	if err != nil {
		return nil, fmt.Errorf("list planner blocks: %w", err)
	}
	blocks := make([]PlannerBlock, len(rows))
	for i, row := range rows {
		blocks[i] = plannerBlockFromRow(row)
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxConcurrentGitLabCalls)
	for i := range blocks {
		if blocks[i].MRIID == nil {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			s.populateProgress(ctx, &blocks[index])
		}(i)
	}
	wg.Wait()

	// Only a fully successful snapshot is cached: an aborted request leaves
	// the previous snapshot in place and the next poll retries.
	s.mu.Lock()
	s.listCache = blocks
	s.listCacheAt = time.Now()
	s.mu.Unlock()
	return blocks, nil
}

// invalidateListCache drops the memoized snapshot after any mutation so the
// next read always reflects the new state.
func (s *Service) invalidateListCache() {
	s.mu.Lock()
	s.listCache = nil
	s.mu.Unlock()
}

// Snapshot returns the plan without calling GitLab. The chat project context
// needs a quick, bounded read; live MR progress is fetched by List instead.
func (s *Service) Snapshot(ctx context.Context) ([]PlannerBlock, error) {
	rows, err := s.store.ListPlannerBlocks(ctx, db.ListPlannerBlocksParams{Limit: publicListLimit})
	if err != nil {
		return nil, fmt.Errorf("snapshot planner blocks: %w", err)
	}
	blocks := make([]PlannerBlock, len(rows))
	for i, row := range rows {
		blocks[i] = plannerBlockFromRow(row)
	}
	return blocks, nil
}

func plannerBlockFromRow(row db.ListPlannerBlocksRow) PlannerBlock {
	return PlannerBlock{
		ID:          row.ID,
		Title:       row.Title,
		Description: row.Description,
		Position:    row.Position,
		Owner: Owner{
			Username:    row.OwnerUsername,
			DisplayName: row.OwnerDisplayName,
			Color:       row.OwnerColor,
		},
		Branch:    row.Branch,
		MRIID:     nullableInt64(row.MergeRequestIid),
		ParentID:  nullableInt64(row.ParentID),
		UpdatedAt: row.UpdatedAt.Time,
		Progress:  initialProgress(row.StartedAt),
	}
}

// ListPublic returns the compact anonymous view. It reuses the same GitLab
// progress calculation but strips every field that is reserved for the team.
func (s *Service) ListPublic(ctx context.Context) ([]PublicPlannerBlock, error) {
	blocks, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	public := make([]PublicPlannerBlock, len(blocks))
	for i, block := range blocks {
		// Anonymous visitors only receive real content for completed stages.
		// Unfinished stages stay in the tree as empty placeholders so the shape
		// of the plan is visible without exposing work in progress.
		title := ""
		updatedAt := time.Time{}
		if block.Progress.State == ProgressDone {
			title = block.Title
			updatedAt = block.UpdatedAt
		}
		public[i] = PublicPlannerBlock{
			ID:        block.ID,
			Title:     title,
			Position:  block.Position,
			ParentID:  block.ParentID,
			UpdatedAt: updatedAt,
			Progress:  PublicProgress{State: block.Progress.State},
		}
	}
	return public, nil
}

func (s *Service) populateProgress(ctx context.Context, block *PlannerBlock) {
	if block.MRIID == nil {
		block.Progress = Progress{State: ProgressPlanned}
		if block.StartedAt != nil {
			block.Progress.State = ProgressVerifying
		}
		return
	}
	if s.gitLab == nil {
		block.Progress = Progress{State: ProgressUnknown, ErrorCode: gitLabRequestError}
		return
	}
	mergeRequest, err := s.gitLab.MergeRequestByIID(ctx, *block.MRIID)
	if err != nil {
		slog.WarnContext(ctx, "planner merge request lookup failed", "block_id", block.ID)
		block.Progress = Progress{State: ProgressUnknown, ErrorCode: gitLabRequestError}
		block.MRURL = ""
		return
	}

	block.MRURL = mergeRequest.WebURL
	block.Progress = progressForMergeRequest(mergeRequest)
	if block.Progress.State == ProgressDone {
		// Block 5: enqueue the auto-pull prompt; block 8: record the chat
		// notification. Both sinks deduplicate their events. The transition
		// must survive an aborted HTTP request, hence WithoutCancel.
		detached := context.WithoutCancel(ctx)
		s.fanOutCompletion(detached, *block, mergeRequest.WebURL)
		s.notifyBlockDone(detached, block)
	}
	if block.Progress.State != ProgressReview {
		return
	}
	approvals, err := s.gitLab.MergeRequestApprovals(ctx, *block.MRIID)
	if err != nil {
		slog.WarnContext(ctx, "planner merge request approvals unavailable", "block_id", block.ID)
		block.Progress.ErrorCode = gitLabApprovalsError
		return
	}
	approvedBy := make([]string, 0, len(approvals.ApprovedBy))
	for _, approval := range approvals.ApprovedBy {
		if approval.User.Username != "" {
			approvedBy = append(approvedBy, approval.User.Username)
		}
	}
	block.Progress.Approvals = &Approvals{
		Approved:      approvals.Approved,
		ApprovalsLeft: approvals.ApprovalsLeft,
		ApprovedBy:    approvedBy,
	}
}

// notifyBlockDone reports an observed merge to the chat notification sink.
// The planner recomputes progress on every read; the sink deduplicates events,
// so repeated observations never create duplicate notifications.
func (s *Service) notifyBlockDone(ctx context.Context, block *PlannerBlock) {
	s.mu.Lock()
	notifier := s.notifier
	s.mu.Unlock()
	if notifier == nil {
		return
	}
	owner := ""
	if block.Owner.Username != "" {
		owner = block.Owner.Username
	}
	notifier.BlockCompleted(ctx, block.ID, block.Title, owner)
}

func initialProgress(startedAt pgtype.Timestamptz) Progress {
	if startedAt.Valid {
		return Progress{State: ProgressVerifying}
	}
	return Progress{State: ProgressPlanned}
}

// fanOutCompletion records the first observation of a merged merge request and
// enqueues exactly one chat change prompt per other team member. The database
// unique constraint on (block_id, username) makes repeated fan-out a no-op, so
// the prompt never spams a member twice.
func (s *Service) fanOutCompletion(ctx context.Context, block PlannerBlock, mrURL string) {
	if _, err := s.store.SetPlannerBlockCompleted(ctx, block.ID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.ErrorContext(ctx, "planner completion transition failed", "block_id", block.ID, "error", err)
		}
		return
	}
	recipients, err := s.store.ListChatPromptRecipients(ctx, block.Owner.Username)
	if err != nil {
		slog.ErrorContext(ctx, "list chat prompt recipients failed", "block_id", block.ID, "error", err)
		return
	}
	for _, username := range recipients {
		if err := s.store.CreateChatPromptIfAbsent(ctx, db.CreateChatPromptIfAbsentParams{
			Username: username, BlockID: block.ID, MrUrl: mrURL,
		}); err != nil {
			slog.ErrorContext(ctx, "create chat change prompt failed", "block_id", block.ID, "username", username, "error", err)
		}
	}
}

func progressForMergeRequest(mergeRequest gitlab.MergeRequest) Progress {
	progress := Progress{PipelineStatus: strings.ToLower(strings.TrimSpace(mergeRequest.HeadPipeline.Status))}
	mergeRequestState := strings.ToLower(strings.TrimSpace(mergeRequest.State))
	if mergeRequestState == "merged" {
		progress.State = ProgressDone
		return progress
	}

	switch progress.PipelineStatus {
	case "pending", "running", "created", "preparing", "scheduled", "waiting_for_resource":
		progress.State = ProgressVerifying
		return progress
	case "failed", "canceled":
		progress.State = ProgressFailed
		return progress
	case "success":
		if mergeRequestState == "opened" {
			progress.State = ProgressReview
			return progress
		}
	}

	progress.State = ProgressUnknown
	progress.ErrorCode = gitLabProgressError
	return progress
}

// Start marks a block as in progress. A team member may start only a block
// they own; the AI worker uses the same ownership check through StartForOwner.
func (s *Service) Start(ctx context.Context, id int64) (PlannerBlock, error) {
	s.invalidateListCache()
	principal, err := identity.FromContext(ctx)
	if err != nil {
		return PlannerBlock{}, err
	}
	return s.StartForOwner(ctx, id, principal.Account.Username)
}

func (s *Service) StartForOwner(ctx context.Context, id int64, username string) (PlannerBlock, error) {
	s.invalidateListCache()
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.store.GetPlannerBlock(ctx, id)
	if err != nil {
		return PlannerBlock{}, fmt.Errorf("get planner block %d: %w", id, classifyMutationError(err))
	}
	owner, err := s.owner(ctx, existing.OwnerID)
	if err != nil {
		return PlannerBlock{}, err
	}
	if owner.Username != username {
		return PlannerBlock{}, huma.Error403Forbidden("planner.owner_required")
	}
	started, err := s.store.StartPlannerBlock(ctx, id)
	if err != nil {
		return PlannerBlock{}, fmt.Errorf("start planner block %d: %w", id, classifyMutationError(err))
	}
	block := plannerBlockFromModel(started, owner)
	s.populateProgress(ctx, &block)
	return block, nil
}

// ListOwned returns the current user's blocks for the coding prompt. It does
// not query GitLab, because the prompt only needs stable IDs and titles.
func (s *Service) ListOwned(ctx context.Context, username string) ([]PlannerBlock, error) {
	rows, err := s.store.ListPlannerBlocks(ctx, db.ListPlannerBlocksParams{Limit: publicListLimit})
	if err != nil {
		return nil, fmt.Errorf("list owned planner blocks: %w", err)
	}
	blocks := make([]PlannerBlock, 0)
	for _, row := range rows {
		if row.OwnerUsername != username {
			continue
		}
		blocks = append(blocks, PlannerBlock{
			ID:          row.ID,
			Title:       row.Title,
			Description: row.Description,
			Position:    row.Position,
			Owner: Owner{
				Username:    row.OwnerUsername,
				DisplayName: row.OwnerDisplayName,
				Color:       row.OwnerColor,
			},
			Branch:    row.Branch,
			MRIID:     nullableInt64(row.MergeRequestIid),
			ParentID:  nullableInt64(row.ParentID),
			StartedAt: nullableTime(row.StartedAt),
			UpdatedAt: row.UpdatedAt.Time,
			Progress:  initialProgress(row.StartedAt),
		})
	}
	return blocks, nil
}

// Create authorizes the HTTP actor before allocating a position or writing.
func (s *Service) Create(ctx context.Context, input CreateInput) (PlannerBlock, error) {
	s.invalidateListCache()
	actor, err := identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP)
	if err != nil {
		return PlannerBlock{}, err
	}
	return s.create(ctx, actor, input)
}

// CreateForAI allows a trusted internal caller to act for an already verified
// administrator. The supplied principal must pass exactly the HTTP admin check.
func (s *Service) CreateForAI(ctx context.Context, actor identity.Principal, input CreateInput) (PlannerBlock, error) {
	if !identity.IsAdmin(actor, s.config.AdminUser, s.config.AdminIP) {
		return PlannerBlock{}, huma.Error403Forbidden("auth.admin_vpn_required")
	}
	return s.create(ctx, actor, input)
}

func (s *Service) create(ctx context.Context, actor identity.Principal, input CreateInput) (PlannerBlock, error) {
	s.invalidateListCache()
	input, err := normalizeInput(input)
	if err != nil {
		return PlannerBlock{}, err
	}
	if err := s.validateParent(ctx, 0, input.ParentID); err != nil {
		return PlannerBlock{}, err
	}
	owner, err := s.owner(ctx, input.OwnerID)
	if err != nil {
		return PlannerBlock{}, err
	}

	s.mu.Lock()
	position := int32(0)
	if input.Position != nil {
		position = *input.Position
	} else {
		position, err = s.store.NextPlannerPosition(ctx)
		if err != nil {
			s.mu.Unlock()
			return PlannerBlock{}, fmt.Errorf("allocate planner position: %w", err)
		}
	}
	created, err := s.store.CreatePlannerBlock(ctx, db.CreatePlannerBlockParams{
		Title:           input.Title,
		Description:     input.Description,
		OwnerID:         input.OwnerID,
		Branch:          input.Branch,
		MergeRequestIid: nullablePGInt64(input.MRIID),
		Position:        position,
		CreatedBy:       actor.Session.Username,
		ParentID:        nullablePGInt64(input.ParentID),
	})
	s.mu.Unlock()
	if err != nil {
		return PlannerBlock{}, fmt.Errorf("create planner block: %w", classifyMutationError(err))
	}

	block := plannerBlockFromModel(created, owner)
	s.populateProgress(ctx, &block)
	return block, nil
}

// Update replaces mutable fields. Omitting position preserves the current one;
// omitting branch or mrIid clears those fields, consistent with PUT semantics.
func (s *Service) Update(ctx context.Context, id int64, input CreateInput) (PlannerBlock, error) {
	s.invalidateListCache()
	if _, err := identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP); err != nil {
		return PlannerBlock{}, err
	}
	input, err := normalizeInput(input)
	if err != nil {
		return PlannerBlock{}, err
	}
	if err := s.validateParent(ctx, id, input.ParentID); err != nil {
		return PlannerBlock{}, err
	}

	s.mu.Lock()
	existing, err := s.store.GetPlannerBlock(ctx, id)
	if err != nil {
		s.mu.Unlock()
		return PlannerBlock{}, fmt.Errorf("get planner block %d: %w", id, classifyMutationError(err))
	}
	owner, err := s.owner(ctx, input.OwnerID)
	if err != nil {
		s.mu.Unlock()
		return PlannerBlock{}, err
	}
	position := existing.Position
	if input.Position != nil {
		position = *input.Position
	}
	updated, err := s.store.UpdatePlannerBlock(ctx, db.UpdatePlannerBlockParams{
		ID:          id,
		Title:       input.Title,
		Description: input.Description,
		OwnerID:     input.OwnerID,
		Position:    position,
		ParentID:    nullablePGInt64(input.ParentID),
	})
	if err == nil && (existing.Branch != input.Branch || !equalOptionalInt64(existing.MergeRequestIid, input.MRIID)) {
		err = s.store.SetPlannerBlockMergeRequest(ctx, db.SetPlannerBlockMergeRequestParams{
			ID:              id,
			Branch:          input.Branch,
			MergeRequestIid: nullablePGInt64(input.MRIID),
		})
	}
	s.mu.Unlock()
	if err != nil {
		return PlannerBlock{}, fmt.Errorf("update planner block %d: %w", id, classifyMutationError(err))
	}

	updated.Branch = input.Branch
	updated.MergeRequestIid = nullablePGInt64(input.MRIID)
	updated.ParentID = nullablePGInt64(input.ParentID)
	block := plannerBlockFromModel(updated, owner)
	s.populateProgress(ctx, &block)
	return block, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	s.invalidateListCache()
	if _, err := identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.store.GetPlannerBlock(ctx, id); err != nil {
		return fmt.Errorf("get planner block %d: %w", id, classifyMutationError(err))
	}
	if err := s.store.DeletePlannerBlock(ctx, id); err != nil {
		return fmt.Errorf("delete planner block %d: %w", id, classifyMutationError(err))
	}
	return nil
}

// validateParent keeps the plan a tree: the parent must exist and following
// the parent chain must never return to the block being edited.
func (s *Service) validateParent(ctx context.Context, id int64, parentID *int64) error {
	seen := map[int64]bool{}
	for parentID != nil {
		if *parentID < 1 || *parentID == id || seen[*parentID] {
			return ErrInvalidInput
		}
		seen[*parentID] = true
		parent, err := s.store.GetPlannerBlock(ctx, *parentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidInput
		}
		if err != nil {
			return fmt.Errorf("get planner parent %d: %w", *parentID, err)
		}
		parentID = nullableInt64(parent.ParentID)
	}
	return nil
}

func (s *Service) owner(ctx context.Context, id int64) (db.LocalAccount, error) {
	owner, err := s.store.GetLocalAccountByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.LocalAccount{}, ErrOwnerNotFound
	}
	if err != nil {
		return db.LocalAccount{}, fmt.Errorf("get planner owner %d: %w", id, err)
	}
	return owner, nil
}

func plannerBlockFromModel(model db.PlannerBlock, owner db.LocalAccount) PlannerBlock {
	return PlannerBlock{
		ID:          model.ID,
		Title:       model.Title,
		Description: model.Description,
		Position:    model.Position,
		Owner: Owner{
			Username:    owner.Username,
			DisplayName: owner.DisplayName,
			Color:       owner.Color,
		},
		Branch:    model.Branch,
		MRIID:     nullableInt64(model.MergeRequestIid),
		ParentID:  nullableInt64(model.ParentID),
		StartedAt: nullableTime(model.StartedAt),
		UpdatedAt: model.UpdatedAt.Time,
		Progress:  initialProgress(model.StartedAt),
	}
}

func normalizeInput(input CreateInput) (CreateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Branch = strings.TrimSpace(input.Branch)
	if input.ParentID != nil && *input.ParentID == 0 {
		input.ParentID = nil
	}
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 200 ||
		utf8.RuneCountInString(input.Description) > 20000 || utf8.RuneCountInString(input.Branch) > 255 ||
		input.OwnerID < 1 || (input.MRIID != nil && *input.MRIID < 1) || (input.Position != nil && *input.Position < 0) {
		return CreateInput{}, ErrInvalidInput
	}
	return input, nil
}

func classifyMutationError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "23505":
			return ErrPositionConflict
		case "23503":
			return ErrOwnerNotFound
		}
	}
	return err
}

func nullableInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func nullablePGInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func equalOptionalInt64(stored pgtype.Int8, input *int64) bool {
	if stored.Valid != (input != nil) {
		return false
	}
	return !stored.Valid || stored.Int64 == *input
}
