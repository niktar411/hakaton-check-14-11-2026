package operator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/llm"
	"hackathon/internal/sam/guard"
)

type fakeModel struct {
	ready    bool
	response llm.Response
	err      error
	complete func(context.Context, llm.Request) (llm.Response, error)
	requests []llm.Request
}

func (f *fakeModel) Ready(context.Context) bool { return f.ready }
func (f *fakeModel) Complete(ctx context.Context, input llm.Request) (llm.Response, error) {
	f.requests = append(f.requests, input)
	if f.complete != nil {
		return f.complete(ctx, input)
	}
	return f.response, f.err
}

type fakeStore struct {
	Store
	conversations []db.SamAiConversation
	messages      []db.SamAiMessage
	nextConvID    int64
	products      []db.SamProduct
	installations []db.SamInstallationsByRawNamesRow
	aliases       []db.SamProductAlias
	licenses      []db.SamLicense
	actions       map[int64]db.SamAiAction
	audits        []string
	pendingAudit  map[int64]string
	nextActionID  int64
	nextAliasID   int64
	businessWrite int
	applyCalls    int
	failApply     error
}

func newFakeStore() *fakeStore {
	return &fakeStore{actions: map[int64]db.SamAiAction{}, pendingAudit: map[int64]string{}}
}

func nowStamp() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

func (f *fakeStore) CreateSamAIConversation(_ context.Context, arg db.CreateSamAIConversationParams) (db.SamAiConversation, error) {
	f.nextConvID++
	row := db.SamAiConversation{ID: f.nextConvID, Actor: arg.Actor, Mode: arg.Mode, CreatedAt: nowStamp(), UpdatedAt: nowStamp()}
	f.conversations = append(f.conversations, row)
	return row, nil
}

func (f *fakeStore) GetSamAIConversation(_ context.Context, arg db.GetSamAIConversationParams) (db.SamAiConversation, error) {
	for _, row := range f.conversations {
		if row.ID == arg.ID && row.Actor == arg.Actor && row.Mode == arg.Mode {
			return row, nil
		}
	}
	return db.SamAiConversation{}, pgx.ErrNoRows
}

func (f *fakeStore) ListSamAIMessages(context.Context, int64) ([]db.SamAiMessage, error) {
	return append([]db.SamAiMessage(nil), f.messages...), nil
}

func (f *fakeStore) AddSamAIMessage(_ context.Context, arg db.AddSamAIMessageParams) error {
	f.messages = append(f.messages, db.SamAiMessage{ConversationID: arg.ConversationID, Role: arg.Role, Content: arg.Content})
	return nil
}

func (f *fakeStore) TouchSamAIConversation(context.Context, int64) error { return nil }

func (f *fakeStore) CreateSamAIAction(_ context.Context, arg db.CreateSamAIActionParams) (db.SamAiAction, error) {
	f.nextActionID++
	conversation := pgtype.Int8{}
	if arg.ConversationID.Valid {
		conversation = arg.ConversationID
	}
	row := db.SamAiAction{
		ID: f.nextActionID, ConversationID: conversation, Actor: arg.Actor, Intent: arg.Intent,
		Payload: append([]byte(nil), arg.Payload...), Preview: append([]byte(nil), arg.Preview...),
		Status: "proposed", Affected: arg.Affected, Fingerprint: arg.Fingerprint, CreatedAt: nowStamp(),
	}
	f.actions[row.ID] = row
	return row, nil
}

func (f *fakeStore) GetSamAIAction(_ context.Context, id int64) (db.SamAiAction, error) {
	row, ok := f.actions[id]
	if !ok {
		return db.SamAiAction{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeStore) GetSamAIActionForUpdate(ctx context.Context, id int64) (db.SamAiAction, error) {
	return f.GetSamAIAction(ctx, id)
}

func (f *fakeStore) ConfirmSamAIAction(_ context.Context, id int64) (int64, error) {
	row, ok := f.actions[id]
	if !ok || row.Status != "proposed" {
		return 0, nil
	}
	row.Status = "confirmed"
	f.actions[id] = row
	return 1, nil
}

func (f *fakeStore) ApplySamAIAction(_ context.Context, arg db.ApplySamAIActionParams) (int64, error) {
	if f.failApply != nil {
		return 0, f.failApply
	}
	row, ok := f.actions[arg.ID]
	if !ok || row.Status != "confirmed" {
		return 0, nil
	}
	row.Status = "applied"
	row.Affected = arg.Affected
	row.Before = append([]byte(nil), arg.Before...)
	row.After = append([]byte(nil), arg.After...)
	row.AppliedAt = nowStamp()
	f.actions[arg.ID] = row
	f.applyCalls++
	return 1, nil
}

func (f *fakeStore) FailSamAIAction(_ context.Context, arg db.FailSamAIActionParams) (int64, error) {
	row, ok := f.actions[arg.ID]
	if !ok {
		return 0, nil
	}
	row.Status = "failed"
	row.Error = arg.Error
	f.actions[arg.ID] = row
	return 1, nil
}

func (f *fakeStore) CancelSamAIAction(_ context.Context, id int64) (int64, error) {
	row, ok := f.actions[id]
	if !ok || row.Status != "proposed" {
		return 0, nil
	}
	row.Status = "cancelled"
	f.actions[id] = row
	return 1, nil
}

func (f *fakeStore) UndoSamAIAction(_ context.Context, id int64) (int64, error) {
	row, ok := f.actions[id]
	if !ok || row.Status != "applied" {
		return 0, nil
	}
	row.Status = "cancelled"
	row.Error = "undone"
	row.UndoneAt = nowStamp()
	f.actions[id] = row
	return 1, nil
}

func (f *fakeStore) ListSamAIActions(_ context.Context, arg db.ListSamAIActionsParams) ([]db.SamAiAction, error) {
	result := make([]db.SamAiAction, 0, len(f.actions))
	for _, row := range f.actions {
		if arg.Actor != "" && row.Actor != arg.Actor {
			continue
		}
		if arg.Status != "" && row.Status != arg.Status {
			continue
		}
		result = append(result, row)
	}
	return result, nil
}

func (f *fakeStore) LatestAppliedSamAIAction(context.Context) (db.SamAiAction, error) {
	var latest db.SamAiAction
	found := false
	for _, row := range f.actions {
		if row.Status != "applied" {
			continue
		}
		if !found || row.ID > latest.ID {
			latest = row
			found = true
		}
	}
	if !found {
		return db.SamAiAction{}, pgx.ErrNoRows
	}
	return latest, nil
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

func (f *fakeStore) SamProductsByRefs(_ context.Context, arg db.SamProductsByRefsParams) ([]db.SamProduct, error) {
	result := make([]db.SamProduct, 0, len(f.products))
	for _, product := range f.products {
		if len(arg.Ids) > 0 && !containsInt(arg.Ids, product.ID) {
			continue
		}
		if arg.Query != "" && !strings.Contains(strings.ToLower(product.Name), strings.ToLower(arg.Query)) &&
			!strings.Contains(strings.ToLower(product.Vendor), strings.ToLower(arg.Query)) {
			continue
		}
		result = append(result, product)
	}
	return result, nil
}

func (f *fakeStore) SamProductsByIDsForUpdate(_ context.Context, ids []int64) ([]db.SamProduct, error) {
	result := make([]db.SamProduct, 0, len(ids))
	for _, product := range f.products {
		if containsInt(ids, product.ID) {
			result = append(result, product)
		}
	}
	return result, nil
}

func (f *fakeStore) SamProductByID(_ context.Context, id int64) (db.SamProduct, error) {
	for _, product := range f.products {
		if product.ID == id {
			return product, nil
		}
	}
	return db.SamProduct{}, pgx.ErrNoRows
}

func (f *fakeStore) SamProductNameExists(_ context.Context, name string) (bool, error) {
	for _, product := range f.products {
		if strings.EqualFold(product.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) SamUpdateProductStatus(_ context.Context, arg db.SamUpdateProductStatusParams) (int64, error) {
	var rows int64
	for index := range f.products {
		if containsInt(arg.Ids, f.products[index].ID) && f.products[index].Status != arg.Status {
			f.products[index].Status = arg.Status
			rows++
		}
	}
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamUpdateProductCategory(_ context.Context, arg db.SamUpdateProductCategoryParams) (int64, error) {
	var rows int64
	for index := range f.products {
		if containsInt(arg.Ids, f.products[index].ID) && f.products[index].Category != arg.Category {
			f.products[index].Category = arg.Category
			rows++
		}
	}
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamArchiveProducts(_ context.Context, ids []int64) (int64, error) {
	var rows int64
	for index := range f.products {
		if containsInt(ids, f.products[index].ID) && f.products[index].Status != "archived" {
			f.products[index].Status = "archived"
			rows++
		}
	}
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamRestoreProductStatuses(_ context.Context, arg db.SamRestoreProductStatusesParams) (int64, error) {
	var rows int64
	for index := range f.products {
		for pos, id := range arg.Ids {
			if f.products[index].ID == id && pos < len(arg.Statuses) {
				f.products[index].Status = arg.Statuses[pos]
				rows++
			}
		}
	}
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamRestoreProductCategories(_ context.Context, arg db.SamRestoreProductCategoriesParams) (int64, error) {
	var rows int64
	for index := range f.products {
		for pos, id := range arg.Ids {
			if f.products[index].ID == id && pos < len(arg.Categories) {
				f.products[index].Category = arg.Categories[pos]
				rows++
			}
		}
	}
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamCreateProduct(_ context.Context, arg db.SamCreateProductParams) (db.SamProduct, error) {
	product := db.SamProduct{ID: int64(len(f.products) + 1), Name: arg.Name, Vendor: arg.Vendor, Version: arg.Version, Category: arg.Category, Status: arg.Status}
	f.products = append(f.products, product)
	f.businessWrite++
	return product, nil
}

func (f *fakeStore) SamDeleteProduct(_ context.Context, id int64) (int64, error) {
	for _, installation := range f.installations {
		if installation.ProductID.Valid && installation.ProductID.Int64 == id {
			return 0, nil
		}
	}
	for _, license := range f.licenses {
		if license.ProductID == id {
			return 0, nil
		}
	}
	for _, alias := range f.aliases {
		if alias.ProductID == id {
			return 0, nil
		}
	}
	for index, product := range f.products {
		if product.ID == id {
			f.products = append(f.products[:index], f.products[index+1:]...)
			f.businessWrite++
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeStore) SamUnmatchedNamesMatching(_ context.Context, arg db.SamUnmatchedNamesMatchingParams) ([]db.SamUnmatchedNamesMatchingRow, error) {
	counts := map[string]int64{}
	devices := map[string]int64{}
	for _, installation := range f.installations {
		if installation.ProductID.Valid {
			continue
		}
		lower := strings.ToLower(installation.RawName)
		matched := false
		for _, pattern := range arg.Patterns {
			if strings.Contains(lower, strings.ToLower(pattern)) {
				matched = true
				break
			}
		}
		if matched {
			counts[installation.RawName]++
			devices[installation.RawName]++
		}
	}
	result := make([]db.SamUnmatchedNamesMatchingRow, 0, len(counts))
	for name, count := range counts {
		result = append(result, db.SamUnmatchedNamesMatchingRow{RawName: name, Installations: count, Devices: devices[name]})
	}
	return result, nil
}

func (f *fakeStore) SamInstallationsByRawNames(_ context.Context, rawNames []string) ([]db.SamInstallationsByRawNamesRow, error) {
	result := make([]db.SamInstallationsByRawNamesRow, 0)
	for _, installation := range f.installations {
		if containsString(rawNames, installation.RawName) && installation.Status == "installed" {
			result = append(result, installation)
		}
	}
	return result, nil
}

func (f *fakeStore) SamInstallationsByRawNamesForUpdate(ctx context.Context, rawNames []string) ([]db.SamInstallationsByRawNamesForUpdateRow, error) {
	rows, err := f.SamInstallationsByRawNames(ctx, rawNames)
	if err != nil {
		return nil, err
	}
	result := make([]db.SamInstallationsByRawNamesForUpdateRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, db.SamInstallationsByRawNamesForUpdateRow{ID: row.ID, ProductID: row.ProductID, RawName: row.RawName, Status: row.Status})
	}
	return result, nil
}

func (f *fakeStore) SamSetInstallationProduct(_ context.Context, arg db.SamSetInstallationProductParams) (int64, error) {
	var rows int64
	for index := range f.installations {
		if containsInt(arg.Ids, f.installations[index].ID) && (!f.installations[index].ProductID.Valid || f.installations[index].ProductID.Int64 != arg.ProductID) {
			f.installations[index].ProductID = pgtype.Int8{Int64: arg.ProductID, Valid: true}
			rows++
		}
	}
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamClearInstallationProduct(_ context.Context, ids []int64) (int64, error) {
	var rows int64
	for index := range f.installations {
		if containsInt(ids, f.installations[index].ID) && f.installations[index].ProductID.Valid {
			f.installations[index].ProductID = pgtype.Int8{}
			rows++
		}
	}
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamAliasConflicts(_ context.Context, normalized []string) ([]db.SamAliasConflictsRow, error) {
	result := make([]db.SamAliasConflictsRow, 0)
	for _, alias := range f.aliases {
		if containsString(normalized, alias.NormalizedAlias) {
			result = append(result, db.SamAliasConflictsRow{NormalizedAlias: alias.NormalizedAlias, ProductID: alias.ProductID})
		}
	}
	return result, nil
}

func (f *fakeStore) SamAliasesForProduct(_ context.Context, productID int64) ([]db.SamProductAlias, error) {
	result := make([]db.SamProductAlias, 0)
	for _, alias := range f.aliases {
		if alias.ProductID == productID {
			result = append(result, alias)
		}
	}
	return result, nil
}

func (f *fakeStore) SamCreateAliases(_ context.Context, arg db.SamCreateAliasesParams) ([]db.SamProductAlias, error) {
	result := make([]db.SamProductAlias, 0, len(arg.Aliases))
	for index, alias := range arg.Aliases {
		normalized := arg.NormalizedAliases[index]
		exists := false
		for _, current := range f.aliases {
			if current.NormalizedAlias == normalized {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		f.nextAliasID++
		row := db.SamProductAlias{ID: f.nextAliasID, ProductID: arg.ProductID, Alias: alias, NormalizedAlias: normalized, CreatedBy: arg.CreatedBy}
		f.aliases = append(f.aliases, row)
		result = append(result, row)
	}
	f.businessWrite += len(result)
	return result, nil
}

func (f *fakeStore) SamDeleteAliases(_ context.Context, ids []int64) (int64, error) {
	var rows int64
	kept := f.aliases[:0]
	for _, alias := range f.aliases {
		if containsInt(ids, alias.ID) {
			rows++
			continue
		}
		kept = append(kept, alias)
	}
	f.aliases = kept
	f.businessWrite += int(rows)
	return rows, nil
}

func (f *fakeStore) SamLicensesByRefs(_ context.Context, arg db.SamLicensesByRefsParams) ([]db.SamLicense, error) {
	result := make([]db.SamLicense, 0)
	for _, license := range f.licenses {
		if len(arg.Ids) > 0 && !containsInt(arg.Ids, license.ID) {
			continue
		}
		if len(arg.ProductIds) > 0 && !containsInt(arg.ProductIds, license.ProductID) {
			continue
		}
		result = append(result, license)
	}
	return result, nil
}

func (f *fakeStore) SamLicensesForUpdate(ctx context.Context, arg db.SamLicensesForUpdateParams) ([]db.SamLicense, error) {
	return f.SamLicensesByRefs(ctx, db.SamLicensesByRefsParams{Ids: arg.Ids, ProductIds: arg.ProductIds})
}

func (f *fakeStore) SamUpdateLicense(_ context.Context, arg db.SamUpdateLicenseParams) (int64, error) {
	for index := range f.licenses {
		if f.licenses[index].ID != arg.ID {
			continue
		}
		if f.licenses[index].LicenseType == arg.LicenseType && f.licenses[index].Seats == arg.Seats &&
			f.licenses[index].CostPerSeat == arg.CostPerSeat && f.licenses[index].Currency == arg.Currency &&
			f.licenses[index].StartsAt == arg.StartsAt && f.licenses[index].ExpiresAt == arg.ExpiresAt &&
			f.licenses[index].Status == arg.Status {
			return 0, nil
		}
		f.licenses[index].LicenseType = arg.LicenseType
		f.licenses[index].Seats = arg.Seats
		f.licenses[index].CostPerSeat = arg.CostPerSeat
		f.licenses[index].Currency = arg.Currency
		f.licenses[index].StartsAt = arg.StartsAt
		f.licenses[index].ExpiresAt = arg.ExpiresAt
		f.licenses[index].Status = arg.Status
		f.businessWrite++
		return 1, nil
	}
	return 0, nil
}

type fakeSnapshot struct {
	conversations []db.SamAiConversation
	messages      []db.SamAiMessage
	products      []db.SamProduct
	installations []db.SamInstallationsByRawNamesRow
	aliases       []db.SamProductAlias
	licenses      []db.SamLicense
	actions       map[int64]db.SamAiAction
	audits        []string
	businessWrite int
	applyCalls    int
}

func (f *fakeStore) snapshot() fakeSnapshot {
	actions := make(map[int64]db.SamAiAction, len(f.actions))
	for key, value := range f.actions {
		actions[key] = value
	}
	return fakeSnapshot{
		conversations: append([]db.SamAiConversation(nil), f.conversations...),
		messages:      append([]db.SamAiMessage(nil), f.messages...),
		products:      append([]db.SamProduct(nil), f.products...),
		installations: append([]db.SamInstallationsByRawNamesRow(nil), f.installations...),
		aliases:       append([]db.SamProductAlias(nil), f.aliases...),
		licenses:      append([]db.SamLicense(nil), f.licenses...),
		actions:       actions,
		audits:        append([]string(nil), f.audits...),
		businessWrite: f.businessWrite,
		applyCalls:    f.applyCalls,
	}
}

func (f *fakeStore) restore(snapshot fakeSnapshot) {
	f.conversations = snapshot.conversations
	f.messages = snapshot.messages
	f.products = snapshot.products
	f.installations = snapshot.installations
	f.aliases = snapshot.aliases
	f.licenses = snapshot.licenses
	f.actions = snapshot.actions
	f.audits = snapshot.audits
	f.businessWrite = snapshot.businessWrite
	f.applyCalls = snapshot.applyCalls
}

func testOperator(store *fakeStore, model *fakeModel) *Service {
	return newService(store, model, guard.NewLimiter(2, 1), func(ctx context.Context, fn func(Store) error) error {
		before := store.snapshot()
		if err := fn(store); err != nil {
			store.restore(before)
			return err
		}
		return nil
	}, Config{})
}

func containsInt(values []int64, value int64) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func chromeStore() *fakeStore {
	store := newFakeStore()
	store.products = []db.SamProduct{{ID: 1, Name: "Google Chrome", Vendor: "Google", Status: "active"}}
	for index, name := range []string{"Chromium", "Google Chrome", "chrome.exe"} {
		store.installations = append(store.installations, db.SamInstallationsByRawNamesRow{ID: int64(index + 1), RawName: name, Status: "installed"})
	}
	return store
}

func toolResponse(name, arguments string) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: name, Arguments: arguments}}}}}
}

func answerResponse(content string) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", Content: content}}
}

func TestProposeMatchAliasesDoesNotWriteAndRequiresPhrase(t *testing.T) {
	store := chromeStore()
	service := testOperator(store, &fakeModel{})
	action, err := service.Propose(context.Background(), 0, "niktar411", "match_aliases", `{"product":"Google Chrome","all":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if action.Status != "proposed" {
		t.Fatalf("expected proposed action, got %s", action.Status)
	}
	if action.Affected != 6 {
		t.Fatalf("expected 3 installations and 3 aliases, got %d", action.Affected)
	}
	if !action.Preview.RequiresPhrase || action.Preview.ConfirmationPhrase != allProductsPhrase {
		t.Fatal("all-products preview must require a confirmation phrase")
	}
	if store.businessWrite != 0 {
		t.Fatalf("preview changed business tables: %d writes", store.businessWrite)
	}
	if len(store.aliases) != 0 {
		t.Fatal("preview created aliases")
	}
	for _, installation := range store.installations {
		if installation.ProductID.Valid {
			t.Fatal("preview mapped an installation")
		}
	}
	if len(action.Preview.Samples) == 0 || len(action.Preview.Diffs) == 0 {
		t.Fatal("preview must include samples and a diff")
	}
}

func TestApplyWritesOnceAndUndoRestores(t *testing.T) {
	store := chromeStore()
	service := testOperator(store, &fakeModel{})
	action, err := service.Propose(context.Background(), 0, "niktar411", "match_aliases", `{"product":"Google Chrome","all":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background(), "niktar411", true, action.ID, "wrong"); err == nil {
		t.Fatal("expected confirmation phrase error")
	} else {
		requireStatus(t, err, 422)
	}
	applied, err := service.Apply(context.Background(), "niktar411", true, action.ID, allProductsPhrase)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != "applied" || applied.Affected != 6 {
		t.Fatalf("unexpected applied action: %+v", applied)
	}
	if len(store.aliases) != 3 {
		t.Fatalf("expected 3 aliases, got %d", len(store.aliases))
	}
	for _, installation := range store.installations {
		if !installation.ProductID.Valid || installation.ProductID.Int64 != 1 {
			t.Fatalf("installation %d was not mapped", installation.ID)
		}
	}
	repeated, err := service.Apply(context.Background(), "niktar411", true, action.ID, allProductsPhrase)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Status != "applied" || store.applyCalls != 1 {
		t.Fatalf("apply must be idempotent, calls=%d", store.applyCalls)
	}
	undone, err := service.Undo(context.Background(), "niktar411", action.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.Status != "cancelled" || len(store.aliases) != 0 {
		t.Fatalf("undo did not restore aliases: %+v", undone)
	}
	for _, installation := range store.installations {
		if installation.ProductID.Valid {
			t.Fatal("undo did not clear installation product")
		}
	}
	if len(store.audits) < 2 {
		t.Fatalf("expected apply and undo audit records, got %v", store.audits)
	}
}

func TestApplyRejectsStalePreview(t *testing.T) {
	store := newFakeStore()
	store.products = []db.SamProduct{{ID: 1, Name: "Notepad++", Status: "active"}}
	service := testOperator(store, &fakeModel{})
	action, err := service.Propose(context.Background(), 0, "niktar411", "set_status", `{"productIds":[1],"status":"outdated"}`)
	if err != nil {
		t.Fatal(err)
	}
	store.products[0].Status = "outdated"
	_, err = service.Apply(context.Background(), "niktar411", true, action.ID, "")
	requireStatus(t, err, 409)
	current, err := store.GetSamAIAction(context.Background(), action.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "proposed" {
		t.Fatalf("stale apply must not consume the action, status=%s", current.Status)
	}
	if store.businessWrite != 0 {
		t.Fatal("stale apply wrote business data")
	}
}

func TestApplyRequiresAdmin(t *testing.T) {
	store := newFakeStore()
	store.products = []db.SamProduct{{ID: 1, Name: "Notepad++", Status: "active"}}
	service := testOperator(store, &fakeModel{})
	action, err := service.Propose(context.Background(), 0, "member", "set_status", `{"productIds":[1],"status":"outdated"}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Apply(context.Background(), "member", false, action.ID, "")
	requireStatus(t, err, 403)
}

func TestProposeRejectsMoreThanLimit(t *testing.T) {
	store := newFakeStore()
	store.products = []db.SamProduct{{ID: 1, Name: "Google Chrome", Status: "active"}}
	for index := 0; index < maxRows+1; index++ {
		store.installations = append(store.installations, db.SamInstallationsByRawNamesRow{ID: int64(index + 1), RawName: "chromium", Status: "installed"})
	}
	service := testOperator(store, &fakeModel{})
	_, err := service.Propose(context.Background(), 0, "niktar411", "match_aliases", `{"product":"Google Chrome"}`)
	requireStatus(t, err, 422)
	if !strings.Contains(err.Error(), "operator.limit_exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.businessWrite != 0 {
		t.Fatal("limit rejection wrote data")
	}
}

func TestChatUsesProposeToolOnly(t *testing.T) {
	store := newFakeStore()
	store.products = []db.SamProduct{{ID: 1, Name: "Notepad++", Status: "active"}}
	calls := 0
	model := &fakeModel{ready: true, complete: func(_ context.Context, input llm.Request) (llm.Response, error) {
		calls++
		if calls == 1 {
			return toolResponse("propose_archive_product", `{"query":"Notepad++"}`), nil
		}
		if len(input.Messages) == 0 || input.Messages[len(input.Messages)-1].Role != "tool" {
			t.Fatal("expected the previous tool result in the model context")
		}
		return answerResponse("Предложил архивирование Notepad++."), nil
	}}
	service := testOperator(store, model)
	result, err := service.Chat(context.Background(), "niktar411", 0, "Архивируй Notepad++", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Actions) != 1 {
		t.Fatalf("unexpected chat result: %+v", result)
	}
	if result.Actions[0].Intent != "archive_product" || result.Actions[0].Status != "proposed" {
		t.Fatalf("unexpected action: %+v", result.Actions[0])
	}
	if store.businessWrite != 0 {
		t.Fatal("chat proposal wrote business data")
	}
	if len(model.requests[0].Tools) != 6 {
		t.Fatalf("expected 6 propose tools, got %d", len(model.requests[0].Tools))
	}
}

func TestFallbackMatchesAllChromes(t *testing.T) {
	store := chromeStore()
	service := testOperator(store, &fakeModel{ready: false})
	result, err := service.Chat(context.Background(), "niktar411", 0, "сопоставь все хромы с Google Chrome", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback || len(result.Actions) != 1 {
		t.Fatalf("unexpected fallback result: %+v", result)
	}
	action := result.Actions[0]
	if action.Intent != "match_aliases" || action.Affected != 6 {
		t.Fatalf("unexpected action: %+v", action)
	}
	if !action.Preview.RequiresPhrase {
		t.Fatal("fallback for all chromes must require the confirmation phrase")
	}
	if store.businessWrite != 0 {
		t.Fatal("fallback preview wrote data")
	}
}

func TestFallbackBansProduct(t *testing.T) {
	store := newFakeStore()
	store.products = []db.SamProduct{{ID: 1, Name: "Radmin", Status: "active"}}
	service := testOperator(store, &fakeModel{ready: false})
	result, err := service.Chat(context.Background(), "niktar411", 0, "Поставь Radmin статус запрещённый", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback || len(result.Actions) != 1 {
		t.Fatalf("unexpected fallback result: %+v", result)
	}
	if result.Actions[0].Intent != "set_status" || !strings.Contains(string(store.actions[result.Actions[0].ID].Payload), "banned") {
		t.Fatalf("unexpected banned action: %+v", result.Actions[0])
	}
	if store.businessWrite != 0 {
		t.Fatal("fallback preview wrote business data")
	}
}

func TestUndoOnlyLatestApplied(t *testing.T) {
	store := newFakeStore()
	store.products = []db.SamProduct{
		{ID: 1, Name: "Notepad++", Status: "active"},
		{ID: 2, Name: "Slack", Status: "active"},
	}
	service := testOperator(store, &fakeModel{})
	first, err := service.Propose(context.Background(), 0, "niktar411", "archive_product", `{"productIds":[1]}`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Propose(context.Background(), 0, "niktar411", "archive_product", `{"productIds":[2]}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background(), "niktar411", true, first.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background(), "niktar411", true, second.ID, ""); err != nil {
		t.Fatal(err)
	}
	_, err = service.Undo(context.Background(), "niktar411", first.ID)
	requireStatus(t, err, 409)
	if _, err := service.Undo(context.Background(), "niktar411", second.ID); err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("expected HTTP %d, got %d (%v)", want, status.GetStatus(), err)
	}
}
