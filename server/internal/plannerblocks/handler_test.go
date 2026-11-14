package plannerblocks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/identity"
)

type fakeStore struct {
	mu          sync.Mutex
	blocks      map[int64]db.PlannerBlock
	accounts    map[int64]db.LocalAccount
	nextID      int64
	listParams  db.ListPlannerBlocksParams
	createCalls []db.CreatePlannerBlockParams
	updateCalls int
	deleteCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		blocks: map[int64]db.PlannerBlock{},
		accounts: map[int64]db.LocalAccount{
			1: {ID: 1, Username: "alice", DisplayName: "Alice Example", Color: "#A1b2C3"},
			2: {ID: 2, Username: "bob", DisplayName: "Bob Example", Color: "#112233"},
		},
		nextID: 1,
	}
}

func (s *fakeStore) ListPlannerBlocks(_ context.Context, arg db.ListPlannerBlocksParams) ([]db.ListPlannerBlocksRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listParams = arg
	rows := make([]db.ListPlannerBlocksRow, 0, len(s.blocks))
	for id, block := range s.blocks {
		account := s.accounts[block.OwnerID]
		rows = append(rows, db.ListPlannerBlocksRow{
			ID:               id,
			Title:            block.Title,
			Description:      block.Description,
			OwnerID:          block.OwnerID,
			Branch:           block.Branch,
			MergeRequestIid:  block.MergeRequestIid,
			Position:         block.Position,
			CreatedBy:        block.CreatedBy,
			CreatedAt:        block.CreatedAt,
			UpdatedAt:        block.UpdatedAt,
			OwnerUsername:    account.Username,
			OwnerDisplayName: account.DisplayName,
			OwnerColor:       account.Color,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Position == rows[j].Position {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Position < rows[j].Position
	})
	start := min(int(arg.Offset), len(rows))
	end := min(start+int(arg.Limit), len(rows))
	result := make([]db.ListPlannerBlocksRow, end-start)
	copy(result, rows[start:end])
	return result, nil
}

func (s *fakeStore) CreatePlannerBlock(_ context.Context, arg db.CreatePlannerBlockParams) (db.PlannerBlock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, block := range s.blocks {
		if block.Position == arg.Position {
			return db.PlannerBlock{}, &pgconn.PgError{Code: "23505", ConstraintName: "planner_blocks_position_idx"}
		}
	}
	s.createCalls = append(s.createCalls, arg)
	block := db.PlannerBlock{
		ID:              s.nextID,
		Title:           arg.Title,
		Description:     arg.Description,
		OwnerID:         arg.OwnerID,
		Branch:          arg.Branch,
		MergeRequestIid: arg.MergeRequestIid,
		Position:        arg.Position,
		CreatedBy:       arg.CreatedBy,
	}
	s.nextID++
	s.blocks[block.ID] = block
	return block, nil
}

func (s *fakeStore) NextPlannerPosition(_ context.Context) (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var next int32
	for _, block := range s.blocks {
		if block.Position >= next {
			next = block.Position + 1
		}
	}
	return next, nil
}

func (s *fakeStore) GetPlannerBlock(_ context.Context, id int64) (db.PlannerBlock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	block, ok := s.blocks[id]
	if !ok {
		return db.PlannerBlock{}, pgx.ErrNoRows
	}
	return block, nil
}

func (s *fakeStore) UpdatePlannerBlock(_ context.Context, arg db.UpdatePlannerBlockParams) (db.PlannerBlock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	block, ok := s.blocks[arg.ID]
	if !ok {
		return db.PlannerBlock{}, pgx.ErrNoRows
	}
	for id, other := range s.blocks {
		if id != arg.ID && other.Position == arg.Position {
			return db.PlannerBlock{}, &pgconn.PgError{Code: "23505", ConstraintName: "planner_blocks_position_idx"}
		}
	}
	s.updateCalls++
	block.Title = arg.Title
	block.Description = arg.Description
	block.OwnerID = arg.OwnerID
	block.Position = arg.Position
	s.blocks[arg.ID] = block
	return block, nil
}

func (s *fakeStore) DeletePlannerBlock(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.blocks, id)
	s.deleteCalls++
	return nil
}

func (s *fakeStore) SetPlannerBlockMergeRequest(_ context.Context, arg db.SetPlannerBlockMergeRequestParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	block, ok := s.blocks[arg.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	block.Branch = arg.Branch
	block.MergeRequestIid = arg.MergeRequestIid
	s.blocks[arg.ID] = block
	return nil
}

func (s *fakeStore) StartPlannerBlock(_ context.Context, id int64) (db.PlannerBlock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	block, ok := s.blocks[id]
	if !ok {
		return db.PlannerBlock{}, pgx.ErrNoRows
	}
	if !block.StartedAt.Valid {
		block.StartedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}
	s.blocks[id] = block
	return block, nil
}

func (s *fakeStore) GetLocalAccountByID(_ context.Context, id int64) (db.LocalAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[id]
	if !ok {
		return db.LocalAccount{}, pgx.ErrNoRows
	}
	return account, nil
}

type fakeGitLab struct {
	mu               sync.Mutex
	mergeRequests    map[int64]gitlab.MergeRequest
	approvals        map[int64]gitlab.Approvals
	mergeRequestErr  error
	approvalsErr     error
	mergeRequestCall int
	approvalsCall    int
}

func (g *fakeGitLab) MergeRequestByIID(_ context.Context, iid int64) (gitlab.MergeRequest, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mergeRequestCall++
	if g.mergeRequestErr != nil {
		return gitlab.MergeRequest{}, g.mergeRequestErr
	}
	mergeRequest, ok := g.mergeRequests[iid]
	if !ok {
		return gitlab.MergeRequest{}, errors.New("unexpected merge request")
	}
	return mergeRequest, nil
}

func (g *fakeGitLab) MergeRequestApprovals(_ context.Context, iid int64) (gitlab.Approvals, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.approvalsCall++
	if g.approvalsErr != nil {
		return gitlab.Approvals{}, g.approvalsErr
	}
	return g.approvals[iid], nil
}

var testConfig = Config{AdminUser: "root", AdminIP: "10.77.0.2"}

func adminPrincipal() identity.Principal {
	return identity.Principal{
		Session:  db.Session{Username: "root"},
		Account:  db.LocalAccount{Username: "root", Role: "admin", Enabled: true},
		AuthKind: "local",
		IP:       "10.77.0.2",
	}
}

func memberPrincipal() identity.Principal {
	principal := adminPrincipal()
	principal.Session.Username = "alice"
	principal.Account.Username = "alice"
	principal.Account.Role = "member"
	return principal
}

func newTestService() (*Service, *fakeStore, *fakeGitLab) {
	store := newFakeStore()
	client := &fakeGitLab{mergeRequests: map[int64]gitlab.MergeRequest{}, approvals: map[int64]gitlab.Approvals{}}
	return newService(store, client, testConfig), store, client
}

func TestProgressForMergeRequest(t *testing.T) {
	tests := []struct {
		name          string
		mergeRequest  string
		pipeline      string
		want          ProgressState
		wantErrorCode bool
	}{
		{name: "merged", mergeRequest: "merged", pipeline: "success", want: ProgressDone},
		{name: "pending", mergeRequest: "opened", pipeline: "pending", want: ProgressVerifying},
		{name: "running", mergeRequest: "opened", pipeline: "running", want: ProgressVerifying},
		{name: "failed", mergeRequest: "opened", pipeline: "failed", want: ProgressFailed},
		{name: "canceled", mergeRequest: "opened", pipeline: "canceled", want: ProgressFailed},
		{name: "successful open", mergeRequest: "opened", pipeline: "success", want: ProgressReview},
		{name: "successful closed", mergeRequest: "closed", pipeline: "success", want: ProgressUnknown, wantErrorCode: true},
		{name: "missing pipeline", mergeRequest: "opened", want: ProgressUnknown, wantErrorCode: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			progress := progressForMergeRequest(gitlab.MergeRequest{
				State:        test.mergeRequest,
				HeadPipeline: gitlab.Pipeline{Status: test.pipeline},
			})
			if progress.State != test.want {
				t.Fatalf("state = %q, want %q", progress.State, test.want)
			}
			if (progress.ErrorCode != "") != test.wantErrorCode {
				t.Fatalf("error code = %q, wantErrorCode %v", progress.ErrorCode, test.wantErrorCode)
			}
		})
	}
}

func TestPublicListIsCompactAndKeepsRealProgress(t *testing.T) {
	svc, store, client := newTestService()
	store.blocks[1] = db.PlannerBlock{ID: 1, Title: "Planned", Description: "Later", OwnerID: 1, Position: 2}
	store.blocks[2] = db.PlannerBlock{
		ID: 2, Title: "Review", Description: "Now", OwnerID: 2, Position: 1,
		Branch: "feature/planner", MergeRequestIid: pgtype.Int8{Int64: 42, Valid: true},
	}
	client.mergeRequests[42] = gitlab.MergeRequest{
		IID: 42, State: "opened", WebURL: "https://gitlab.example/mr/42",
		HeadPipeline: gitlab.Pipeline{Status: "success"},
	}

	_, api := humatest.New(t)
	Register(api, svc)
	response := api.Get("/api/public/planner")
	if response.Code != http.StatusOK {
		t.Fatalf("public GET without authentication: status %d, body %s", response.Code, response.Body.String())
	}
	var blocks []PublicPlannerBlock
	if err := json.Unmarshal(response.Body.Bytes(), &blocks); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if store.listParams.Limit != publicListLimit || store.listParams.Offset != 0 {
		t.Fatalf("list parameters = %+v", store.listParams)
	}
	review := blocks[0]
	if review.Progress.State != ProgressReview {
		t.Fatalf("progress = %+v", review.Progress)
	}
	if encoded := response.Body.String(); strings.Contains(encoded, "bob") ||
		strings.Contains(encoded, "feature/planner") || strings.Contains(encoded, "gitlab.example") {
		t.Fatalf("public response leaked team details: %s", encoded)
	}
	planned := blocks[1]
	if planned.Progress.State != ProgressPlanned {
		t.Fatalf("planned block = %+v", planned)
	}
	if client.mergeRequestCall != 1 {
		t.Fatalf("GitLab called %d times, want only the block with an MR", client.mergeRequestCall)
	}
}

func TestTeamListKeepsFullDetails(t *testing.T) {
	svc, store, client := newTestService()
	store.blocks[1] = db.PlannerBlock{
		ID: 1, Title: "Review", Description: "Now", OwnerID: 2, Position: 1,
		Branch: "feature/planner", MergeRequestIid: pgtype.Int8{Int64: 42, Valid: true},
	}
	client.mergeRequests[42] = gitlab.MergeRequest{
		IID: 42, State: "opened", WebURL: "https://gitlab.example/mr/42",
		HeadPipeline: gitlab.Pipeline{Status: "success"},
	}

	_, api := humatest.New(t)
	Register(api, svc)
	response := api.Get("/api/planner")
	if response.Code != http.StatusOK {
		t.Fatalf("team GET: status %d, body %s", response.Code, response.Body.String())
	}
	var blocks []PlannerBlock
	if err := json.Unmarshal(response.Body.Bytes(), &blocks); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	block := blocks[0]
	if block.Owner.Username != "bob" || block.Owner.DisplayName != "Bob Example" || block.Owner.Color != "#112233" {
		t.Fatalf("owner was not passed through from verified account: %+v", block.Owner)
	}
	if block.MRIID == nil || *block.MRIID != 42 || block.MRURL != "https://gitlab.example/mr/42" {
		t.Fatalf("merge request fields = %+v", block)
	}
	if block.Progress.State != ProgressReview || block.Progress.PipelineStatus != "success" {
		t.Fatalf("progress = %+v", block.Progress)
	}
}

func TestGitLabFailureIsUnknownAndPublicSafe(t *testing.T) {
	svc, store, client := newTestService()
	store.blocks[1] = db.PlannerBlock{
		ID: 1, Title: "Unknown", OwnerID: 1, MergeRequestIid: pgtype.Int8{Int64: 99, Valid: true},
	}
	client.mergeRequestErr = errors.New("request used PRIVATE-TOKEN=super-secret")

	blocks, err := svc.List(t.Context())
	if err != nil {
		t.Fatalf("list returned a GitLab error: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Progress.State != ProgressUnknown || blocks[0].Progress.ErrorCode != gitLabRequestError {
		t.Fatalf("blocks = %+v", blocks)
	}
	encoded, err := json.Marshal(blocks[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "super-secret") || strings.Contains(string(encoded), "PRIVATE-TOKEN") {
		t.Fatalf("upstream details leaked: %s", encoded)
	}
}

func TestApprovalsFailureDoesNotFabricateOrEraseKnownProgress(t *testing.T) {
	svc, store, client := newTestService()
	store.blocks[1] = db.PlannerBlock{
		ID: 1, Title: "Review", OwnerID: 1, MergeRequestIid: pgtype.Int8{Int64: 42, Valid: true},
	}
	client.mergeRequests[42] = gitlab.MergeRequest{State: "opened", HeadPipeline: gitlab.Pipeline{Status: "success"}}
	client.approvalsErr = errors.New("approvals forbidden")

	blocks, err := svc.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if blocks[0].Progress.State != ProgressReview || blocks[0].Progress.Approvals != nil {
		t.Fatalf("progress = %+v", blocks[0].Progress)
	}
}

func TestOwnerCanStartOnlyOwnBlock(t *testing.T) {
	svc, store, _ := newTestService()
	store.blocks[1] = db.PlannerBlock{ID: 1, Title: "Mine", OwnerID: 1}
	store.blocks[2] = db.PlannerBlock{ID: 2, Title: "Other", OwnerID: 2}
	ctx := identity.WithPrincipal(t.Context(), memberPrincipal())

	block, err := svc.Start(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if block.Progress.State != ProgressVerifying || block.StartedAt == nil {
		t.Fatalf("started block progress = %#v", block)
	}
	if _, err := svc.Start(ctx, 2); err == nil {
		t.Fatal("member started another owner's block")
	}
	started, err := svc.StartForOwner(t.Context(), 1, "alice")
	if err != nil || started.Progress.State != ProgressVerifying {
		t.Fatalf("AI start for owner failed: %#v, %v", started, err)
	}
}

func TestAdminRightsAndCreateForAI(t *testing.T) {
	svc, store, _ := newTestService()
	input := CreateInput{Title: "  New block  ", Description: "Description", OwnerID: 1}

	_, err := svc.Create(t.Context(), input)
	requireHTTPStatus(t, err, http.StatusUnauthorized)
	memberCtx := identity.WithPrincipal(t.Context(), memberPrincipal())
	_, err = svc.Create(memberCtx, input)
	requireHTTPStatus(t, err, http.StatusForbidden)
	_, err = svc.Update(memberCtx, 1, input)
	requireHTTPStatus(t, err, http.StatusForbidden)
	err = svc.Delete(memberCtx, 1)
	requireHTTPStatus(t, err, http.StatusForbidden)
	if len(store.createCalls) != 0 || store.updateCalls != 0 || store.deleteCalls != 0 {
		t.Fatal("a non-admin mutation reached storage")
	}

	adminCtx := identity.WithPrincipal(t.Context(), adminPrincipal())
	created, err := svc.Create(adminCtx, input)
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	if created.Title != "New block" || created.Position != 0 || created.Progress.State != ProgressPlanned {
		t.Fatalf("created block = %+v", created)
	}
	if created.Owner.Username != "alice" || created.Owner.Color != "#A1b2C3" {
		t.Fatalf("created owner = %+v", created.Owner)
	}
	if len(store.createCalls) != 1 || store.createCalls[0].CreatedBy != "root" {
		t.Fatalf("created_by was not taken from the principal: %+v", store.createCalls)
	}

	_, err = svc.CreateForAI(t.Context(), memberPrincipal(), CreateInput{Title: "Denied", OwnerID: 1})
	requireHTTPStatus(t, err, http.StatusForbidden)
	aiCreated, err := svc.CreateForAI(t.Context(), adminPrincipal(), CreateInput{Title: "AI block", OwnerID: 2})
	if err != nil {
		t.Fatalf("admin AI create: %v", err)
	}
	if aiCreated.Position != 1 || aiCreated.Owner.Username != "bob" {
		t.Fatalf("AI-created block = %+v", aiCreated)
	}
}

func TestAdminUpdateDeleteAndPositionConflict(t *testing.T) {
	svc, store, client := newTestService()
	store.blocks[7] = db.PlannerBlock{ID: 7, Title: "Old", OwnerID: 1, Position: 3}
	adminCtx := identity.WithPrincipal(t.Context(), adminPrincipal())
	position := int32(4)
	iid := int64(55)
	client.mergeRequests[55] = gitlab.MergeRequest{
		State: "opened", WebURL: "https://gitlab.example/mr/55", HeadPipeline: gitlab.Pipeline{Status: "success"},
	}

	updated, err := svc.Update(adminCtx, 7, CreateInput{
		Title: "Updated", Description: "Verified", OwnerID: 2, Branch: "feature/new", MRIID: &iid, Position: &position,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "Updated" || updated.Owner.Username != "bob" || updated.Branch != "feature/new" ||
		updated.MRIID == nil || *updated.MRIID != 55 || updated.Progress.State != ProgressReview {
		t.Fatalf("updated block = %+v", updated)
	}
	if store.blocks[7].Branch != "feature/new" || store.blocks[7].MergeRequestIid.Int64 != 55 {
		t.Fatalf("stored merge request = %+v", store.blocks[7])
	}

	_, err = svc.Create(adminCtx, CreateInput{Title: "Conflict", OwnerID: 1, Position: &position})
	if !errors.Is(err, ErrPositionConflict) {
		t.Fatalf("duplicate position error = %v, want ErrPositionConflict", err)
	}
	if err := svc.Delete(adminCtx, 7); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := svc.Delete(adminCtx, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
}

func TestAdminRoutesRejectAnonymousRequests(t *testing.T) {
	svc, _, _ := newTestService()
	_, api := humatest.New(t)
	Register(api, svc)
	body := `{"title":"Block","ownerId":1}`

	for _, test := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/admin/planner/blocks", body: body},
		{method: http.MethodPut, path: "/api/admin/planner/blocks/1", body: body},
		{method: http.MethodDelete, path: "/api/admin/planner/blocks/1"},
	} {
		response := api.Do(test.method, test.path, strings.NewReader(test.body), "Content-Type: application/json")
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401; body %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestAdminRoutesCreateUpdateDelete(t *testing.T) {
	svc, _, _ := newTestService()
	_, api := humatest.New(t)
	Register(api, svc)
	adminCtx := identity.WithPrincipal(t.Context(), adminPrincipal())

	response := api.PostCtx(adminCtx, "/api/admin/planner/blocks", map[string]any{
		"title": "HTTP block", "description": "Created", "ownerId": 1,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create status %d, want 201; body %s", response.Code, response.Body.String())
	}
	response = api.PutCtx(adminCtx, "/api/admin/planner/blocks/1", map[string]any{
		"title": "Updated HTTP block", "ownerId": 2,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("update status %d, want 200; body %s", response.Code, response.Body.String())
	}
	var updated PlannerBlock
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Updated HTTP block" || updated.Owner.Username != "bob" {
		t.Fatalf("update response = %+v", updated)
	}
	response = api.DeleteCtx(adminCtx, "/api/admin/planner/blocks/1")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status %d, want 204; body %s", response.Code, response.Body.String())
	}
}

func requireHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()
	var statusError huma.StatusError
	if !errors.As(err, &statusError) || statusError.GetStatus() != want {
		t.Fatalf("error = %v, want HTTP status %d", err, want)
	}
}
