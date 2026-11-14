package matching

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

type fakeStore struct {
	Store
	products      []db.SamMatchingProductsRow
	aliases       []db.SamMatchingAliasesRow
	unmatched     []db.SamMatchingUnmatchedInstallationsRow
	candidates    map[int64]db.SamMatchCandidate
	mapped        map[int64]int64
	audits        []string
	pendingAudit  map[int64]string
	nextCandidate int64
	nextAlias     int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		candidates:   map[int64]db.SamMatchCandidate{},
		mapped:       map[int64]int64{},
		pendingAudit: map[int64]string{},
	}
}

func stamp() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

func (f *fakeStore) SamMatchingProducts(context.Context) ([]db.SamMatchingProductsRow, error) {
	return append([]db.SamMatchingProductsRow(nil), f.products...), nil
}

func (f *fakeStore) SamMatchingAliases(context.Context) ([]db.SamMatchingAliasesRow, error) {
	return append([]db.SamMatchingAliasesRow(nil), f.aliases...), nil
}

func (f *fakeStore) SamMatchingUnmatchedInstallations(_ context.Context, arg db.SamMatchingUnmatchedInstallationsParams) ([]db.SamMatchingUnmatchedInstallationsRow, error) {
	result := make([]db.SamMatchingUnmatchedInstallationsRow, 0)
	for _, row := range f.unmatched {
		if _, mapped := f.mapped[row.ID]; mapped {
			continue
		}
		result = append(result, row)
	}
	if int(arg.Limit) < len(result) {
		result = result[:arg.Limit]
	}
	return result, nil
}

func (f *fakeStore) SamMatchingUpsertCandidate(_ context.Context, arg db.SamMatchingUpsertCandidateParams) (db.SamMatchCandidate, error) {
	for id, candidate := range f.candidates {
		if candidate.InstallationID == arg.InstallationID && candidate.ProductID == arg.ProductID {
			if candidate.Status != "pending" {
				return db.SamMatchCandidate{}, pgx.ErrNoRows
			}
			candidate.RawName = arg.RawName
			candidate.Score = arg.Score
			candidate.Method = arg.Method
			f.candidates[id] = candidate
			return candidate, nil
		}
	}
	f.nextCandidate++
	candidate := db.SamMatchCandidate{
		ID: f.nextCandidate, InstallationID: arg.InstallationID, ProductID: arg.ProductID,
		RawName: arg.RawName, Score: arg.Score, Method: arg.Method, Status: "pending", CreatedAt: stamp(),
	}
	f.candidates[candidate.ID] = candidate
	return candidate, nil
}

func (f *fakeStore) SamMatchingCandidates(_ context.Context, arg db.SamMatchingCandidatesParams) ([]db.SamMatchingCandidatesRow, error) {
	result := make([]db.SamMatchingCandidatesRow, 0)
	for _, candidate := range f.candidates {
		if arg.Status != "" && candidate.Status != arg.Status {
			continue
		}
		view, err := f.view(candidate)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

func (f *fakeStore) SamMatchingCandidateView(_ context.Context, id int64) (db.SamMatchingCandidateViewRow, error) {
	candidate, ok := f.candidates[id]
	if !ok {
		return db.SamMatchingCandidateViewRow{}, pgx.ErrNoRows
	}
	view, err := f.view(candidate)
	if err != nil {
		return db.SamMatchingCandidateViewRow{}, err
	}
	return db.SamMatchingCandidateViewRow(view), nil
}

func (f *fakeStore) view(candidate db.SamMatchCandidate) (db.SamMatchingCandidatesRow, error) {
	productName := ""
	status := ""
	for _, product := range f.products {
		if product.ID == candidate.ProductID {
			productName = product.Name
			status = product.Status
			break
		}
	}
	device := ""
	lastSeen := pgtype.Timestamptz{}
	for _, installation := range f.unmatched {
		if installation.ID == candidate.InstallationID {
			device = installation.DeviceName
			lastSeen = installation.LastSeenAt
			break
		}
	}
	_ = status
	return db.SamMatchingCandidatesRow{
		ID: candidate.ID, InstallationID: candidate.InstallationID, ProductID: candidate.ProductID,
		RawName: candidate.RawName, Score: candidate.Score, Method: candidate.Method, Status: candidate.Status,
		CreatedAt: candidate.CreatedAt, ResolvedAt: candidate.ResolvedAt, ResolvedBy: candidate.ResolvedBy,
		ProductName: productName, DeviceName: device, LastSeenAt: lastSeen,
	}, nil
}

func (f *fakeStore) SamMatchingCandidateForUpdate(_ context.Context, id int64) (db.SamMatchCandidate, error) {
	candidate, ok := f.candidates[id]
	if !ok {
		return db.SamMatchCandidate{}, pgx.ErrNoRows
	}
	return candidate, nil
}

func (f *fakeStore) SamMatchingConfirmCandidate(_ context.Context, arg db.SamMatchingConfirmCandidateParams) (int64, error) {
	candidate, ok := f.candidates[arg.ID]
	if !ok || candidate.Status != "pending" {
		return 0, nil
	}
	candidate.Status = "confirmed"
	candidate.ResolvedAt = stamp()
	candidate.ResolvedBy = arg.ResolvedBy
	f.candidates[arg.ID] = candidate
	return 1, nil
}

func (f *fakeStore) SamMatchingRejectCandidate(_ context.Context, arg db.SamMatchingRejectCandidateParams) (int64, error) {
	candidate, ok := f.candidates[arg.ID]
	if !ok || candidate.Status != "pending" {
		return 0, nil
	}
	candidate.Status = "rejected"
	candidate.ResolvedAt = stamp()
	candidate.ResolvedBy = arg.ResolvedBy
	f.candidates[arg.ID] = candidate
	return 1, nil
}

func (f *fakeStore) SamMatchingRejectOtherCandidates(_ context.Context, arg db.SamMatchingRejectOtherCandidatesParams) (int64, error) {
	var rows int64
	for id, candidate := range f.candidates {
		if candidate.InstallationID == arg.InstallationID && id != arg.ID && candidate.Status == "pending" {
			candidate.Status = "rejected"
			candidate.ResolvedAt = stamp()
			candidate.ResolvedBy = arg.ResolvedBy
			f.candidates[id] = candidate
			rows++
		}
	}
	return rows, nil
}

func (f *fakeStore) SamMatchingAliasProduct(_ context.Context, normalized string) (int64, error) {
	for _, alias := range f.aliases {
		if alias.NormalizedAlias == normalized {
			return alias.ProductID, nil
		}
	}
	return 0, pgx.ErrNoRows
}

func (f *fakeStore) SamMatchingUpsertAlias(_ context.Context, arg db.SamMatchingUpsertAliasParams) (int64, error) {
	for index, alias := range f.aliases {
		if alias.NormalizedAlias != arg.NormalizedAlias {
			continue
		}
		if alias.ProductID != arg.ProductID {
			return 0, nil
		}
		if arg.Confidence > alias.Confidence {
			f.aliases[index].Confidence = arg.Confidence
		}
		return 1, nil
	}
	f.nextAlias++
	f.aliases = append(f.aliases, db.SamMatchingAliasesRow{
		ID: f.nextAlias, ProductID: arg.ProductID, Alias: arg.Alias,
		NormalizedAlias: arg.NormalizedAlias, Confidence: arg.Confidence,
	})
	return 1, nil
}

func (f *fakeStore) SamMatchingSetInstallationProduct(_ context.Context, arg db.SamMatchingSetInstallationProductParams) (int64, error) {
	if !arg.ProductID.Valid {
		return 0, nil
	}
	if _, exists := f.mapped[arg.ID]; exists {
		return 0, nil
	}
	f.mapped[arg.ID] = arg.ProductID.Int64
	return 1, nil
}

func (f *fakeStore) SamMatchingCountUnmatched(context.Context) (int64, error) {
	var count int64
	for _, installation := range f.unmatched {
		if _, mapped := f.mapped[installation.ID]; !mapped {
			count++
		}
	}
	return count, nil
}

func (f *fakeStore) SamMatchingCountPending(context.Context) (int64, error) {
	return f.SamMatchingCountCandidatesByStatus(context.Background(), "pending")
}

func (f *fakeStore) SamMatchingCountCandidatesByStatus(_ context.Context, status string) (int64, error) {
	var count int64
	for _, candidate := range f.candidates {
		if candidate.Status == status {
			count++
		}
	}
	return count, nil
}

func (f *fakeStore) BeginAudit(_ context.Context, arg db.BeginAuditParams) (int64, error) {
	id := int64(len(f.pendingAudit) + 1)
	f.pendingAudit[id] = arg.Action + ":" + arg.Target
	return id, nil
}

func (f *fakeStore) FinishAudit(_ context.Context, arg db.FinishAuditParams) error {
	f.audits = append(f.audits, f.pendingAudit[arg.ID]+"="+arg.Status)
	return nil
}

func testService(store *fakeStore) *Service {
	return newService(store, func(ctx context.Context, fn func(Store) error) error {
		return fn(store)
	})
}

func chromeStore() *fakeStore {
	store := newFakeStore()
	store.products = []db.SamMatchingProductsRow{{ID: 1, Name: "Google Chrome", Vendor: "Google", Status: "active"}}
	for index, raw := range []string{"chrome.exe", "Google_Chrome_v2", "chrome_setup", "Хром экзе", "Google Chrome (x64)"} {
		store.unmatched = append(store.unmatched, db.SamMatchingUnmatchedInstallationsRow{
			ID: int64(index + 1), RawName: raw, DeviceName: "PC-0001", LastSeenAt: stamp(),
		})
	}
	return store
}

func TestRunCreatesCandidatesWithoutChangingInstallations(t *testing.T) {
	store := chromeStore()
	service := testService(store)
	result, err := service.Run(context.Background(), "niktar411", 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 5 || result.Matched != 5 || result.Candidates != 5 || result.Skipped != 0 {
		t.Fatalf("unexpected run stats: %+v", result)
	}
	if len(store.mapped) != 0 {
		t.Fatal("matching run mapped installations before confirmation")
	}
	if len(store.aliases) != 0 {
		t.Fatal("matching run created aliases before confirmation")
	}
	if result.Pending != 5 {
		t.Fatalf("pending candidates = %d, want 5", result.Pending)
	}
}

func TestRunSkipsUnknownNames(t *testing.T) {
	store := newFakeStore()
	store.products = []db.SamMatchingProductsRow{{ID: 1, Name: "Google Chrome", Status: "active"}}
	store.unmatched = []db.SamMatchingUnmatchedInstallationsRow{{ID: 1, RawName: "unknown-zzz", DeviceName: "PC-0001", LastSeenAt: stamp()}}
	service := testService(store)
	result, err := service.Run(context.Background(), "niktar411", 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 0 || result.Skipped != 1 || result.Candidates != 0 {
		t.Fatalf("unexpected run stats: %+v", result)
	}
}

func TestConfirmCreatesAliasAndMapsInstallation(t *testing.T) {
	store := chromeStore()
	service := testService(store)
	if _, err := service.Run(context.Background(), "niktar411", 100); err != nil {
		t.Fatal(err)
	}
	pending, err := service.Candidates(context.Background(), "pending", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 {
		t.Fatal("no candidates")
	}
	candidate, err := service.Confirm(context.Background(), "niktar411", pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != "confirmed" || candidate.ResolvedBy != "niktar411" {
		t.Fatalf("unexpected confirmed candidate: %+v", candidate)
	}
	if len(store.aliases) != 1 {
		t.Fatalf("expected one alias, got %d", len(store.aliases))
	}
	if _, mapped := store.mapped[candidate.InstallationID]; !mapped {
		t.Fatal("installation was not mapped")
	}
	if len(store.audits) != 1 {
		t.Fatalf("expected one audit record, got %v", store.audits)
	}
}

func TestRejectCandidate(t *testing.T) {
	store := chromeStore()
	service := testService(store)
	if _, err := service.Run(context.Background(), "niktar411", 100); err != nil {
		t.Fatal(err)
	}
	pending, err := service.Candidates(context.Background(), "pending", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := service.Reject(context.Background(), "niktar411", pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != "rejected" {
		t.Fatalf("unexpected rejected candidate: %+v", candidate)
	}
	if len(store.aliases) != 0 || len(store.mapped) != 0 {
		t.Fatal("reject changed aliases or installations")
	}
}

func TestConfirmRejectsAliasConflict(t *testing.T) {
	store := chromeStore()
	store.aliases = []db.SamMatchingAliasesRow{{ID: 1, ProductID: 2, Alias: "chrome", NormalizedAlias: "chrome", Confidence: 1}}
	service := testService(store)
	if _, err := service.Run(context.Background(), "niktar411", 100); err != nil {
		t.Fatal(err)
	}
	pending, err := service.Candidates(context.Background(), "pending", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	target := int64(0)
	for _, candidate := range pending {
		if Normalize(candidate.RawName) == "chrome" {
			target = candidate.ID
			break
		}
	}
	if target == 0 {
		t.Fatal("chrome candidate not found")
	}
	_, err = service.Confirm(context.Background(), "niktar411", target)
	requireStatus(t, err, 409)
}

func TestConfirmRejectsAlreadyResolvedCandidate(t *testing.T) {
	store := chromeStore()
	service := testService(store)
	if _, err := service.Run(context.Background(), "niktar411", 100); err != nil {
		t.Fatal(err)
	}
	pending, err := service.Candidates(context.Background(), "pending", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "niktar411", pending[0].ID); err != nil {
		t.Fatal(err)
	}
	_, err = service.Confirm(context.Background(), "niktar411", pending[0].ID)
	requireStatus(t, err, 409)
}

func requireStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected HTTP %d, got nil", want)
	}
	var status interface{ GetStatus() int }
	if !errors.As(err, &status) {
		t.Fatalf("expected status error, got %v", err)
	}
	if status.GetStatus() != want {
		t.Fatalf("expected HTTP %d, got %d", want, status.GetStatus())
	}
}
