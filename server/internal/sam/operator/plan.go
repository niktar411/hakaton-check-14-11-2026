package operator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

const allProductsPhrase = "ПОДТВЕРЖДАЮ ИЗМЕНЕНИЕ ВСЕХ"

type Plan struct {
	Intent         string
	Summary        string
	Affected       int
	Samples        []map[string]any
	Diffs          []SamOperatorDiff
	Warnings       []string
	RequiresPhrase bool
	Phrase         string
	Fingerprint    string
	Apply          func(ctx context.Context, store Store) (ExecResult, error)
}

type ExecResult struct {
	Before   any
	After    any
	Affected int
}

type setStatusArgs struct {
	ProductIDs []int64 `json:"productIds"`
	Query      string  `json:"query"`
	All        bool    `json:"all"`
	Status     string  `json:"status"`
}

type setCategoryArgs struct {
	ProductIDs []int64 `json:"productIds"`
	Query      string  `json:"query"`
	All        bool    `json:"all"`
	Category   string  `json:"category"`
}

type archiveProductArgs struct {
	ProductIDs []int64 `json:"productIds"`
	Query      string  `json:"query"`
	All        bool    `json:"all"`
}

type matchAliasesArgs struct {
	Product string   `json:"product"`
	Aliases []string `json:"aliases"`
	All     bool     `json:"all"`
}

type createProductArgs struct {
	Name     string   `json:"name"`
	Vendor   string   `json:"vendor"`
	Version  string   `json:"version"`
	Category string   `json:"category"`
	Status   string   `json:"status"`
	Aliases  []string `json:"aliases"`
	All      bool     `json:"all"`
}

type updateLicenseArgs struct {
	LicenseIDs  []int64 `json:"licenseIds"`
	Product     string  `json:"product"`
	LicenseType *string `json:"licenseType"`
	Seats       *int64  `json:"seats"`
	CostPerSeat *int64  `json:"costPerSeat"`
	Currency    *string `json:"currency"`
	StartsAt    *string `json:"startsAt"`
	ExpiresAt   *string `json:"expiresAt"`
	Status      *string `json:"status"`
	All         bool    `json:"all"`
}

type productValue struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Category string `json:"category"`
}

type productChange struct {
	Products []productValue `json:"products"`
}

type installationValue struct {
	ID        int64  `json:"id"`
	RawName   string `json:"rawName"`
	ProductID *int64 `json:"productId"`
}

type installationChange struct {
	Installations []installationValue `json:"installations"`
}

type aliasValue struct {
	ID              int64  `json:"id"`
	Alias           string `json:"alias"`
	NormalizedAlias string `json:"normalizedAlias"`
}

type matchBefore struct {
	Aliases       []aliasValue        `json:"aliases"`
	Installations []installationValue `json:"installations"`
}

type matchAfter struct {
	Aliases       []aliasValue        `json:"aliases"`
	Installations []installationValue `json:"installations"`
}

type licenseValue struct {
	ID          int64  `json:"id"`
	ProductID   int64  `json:"productId"`
	LicenseType string `json:"licenseType"`
	Seats       int64  `json:"seats"`
	CostPerSeat int64  `json:"costPerSeat"`
	Currency    string `json:"currency"`
	StartsAt    string `json:"startsAt"`
	ExpiresAt   string `json:"expiresAt"`
	Status      string `json:"status"`
}

type licenseChange struct {
	Licenses []licenseValue `json:"licenses"`
}

type productCreated struct {
	Product productValue `json:"product"`
	Aliases []aliasValue `json:"aliases"`
}

func buildPlan(ctx context.Context, store Store, intent string, raw []byte) (*Plan, error) {
	switch intent {
	case "match_aliases":
		var args matchAliasesArgs
		if err := decodeStrict(raw, &args); err != nil {
			return nil, huma.Error400BadRequest("operator.invalid_arguments")
		}
		return buildMatchAliasesPlan(ctx, store, args)
	case "set_status":
		var args setStatusArgs
		if err := decodeStrict(raw, &args); err != nil {
			return nil, huma.Error400BadRequest("operator.invalid_arguments")
		}
		return buildStatusPlan(ctx, store, args, "set_status")
	case "archive_product":
		var args archiveProductArgs
		if err := decodeStrict(raw, &args); err != nil {
			return nil, huma.Error400BadRequest("operator.invalid_arguments")
		}
		return buildStatusPlan(ctx, store, setStatusArgs{ProductIDs: args.ProductIDs, Query: args.Query, All: args.All, Status: "archived"}, "archive_product")
	case "set_category":
		var args setCategoryArgs
		if err := decodeStrict(raw, &args); err != nil {
			return nil, huma.Error400BadRequest("operator.invalid_arguments")
		}
		return buildCategoryPlan(ctx, store, args)
	case "update_license":
		var args updateLicenseArgs
		if err := decodeStrict(raw, &args); err != nil {
			return nil, huma.Error400BadRequest("operator.invalid_arguments")
		}
		return buildLicensePlan(ctx, store, args)
	case "create_product":
		var args createProductArgs
		if err := decodeStrict(raw, &args); err != nil {
			return nil, huma.Error400BadRequest("operator.invalid_arguments")
		}
		return buildCreateProductPlan(ctx, store, args)
	default:
		return nil, huma.Error422UnprocessableEntity("operator.unknown_intent")
	}
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("multiple JSON values")
	}
	return nil
}

func fingerprintOf(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode fingerprint: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func phraseFor(all bool) (bool, string) {
	if all {
		return true, allProductsPhrase
	}
	return false, ""
}

func resolveProductSingle(ctx context.Context, store Store, query string) (db.SamProduct, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return db.SamProduct{}, huma.Error422UnprocessableEntity("operator.product_required")
	}
	rows, err := store.SamProductsByRefs(ctx, db.SamProductsByRefsParams{Ids: []int64{}, Query: query, Limit: 20})
	if err != nil {
		return db.SamProduct{}, fmt.Errorf("resolve SAM product: %w", err)
	}
	if len(rows) == 0 {
		return db.SamProduct{}, huma.Error404NotFound("operator.product_not_found")
	}
	for _, row := range rows {
		if strings.EqualFold(row.Name, query) {
			return row, nil
		}
	}
	if len(rows) == 1 {
		return rows[0], nil
	}
	return db.SamProduct{}, huma.Error409Conflict("operator.ambiguous_product")
}

func resolveProducts(ctx context.Context, store Store, ids []int64, query string, all bool) ([]db.SamProduct, error) {
	query = strings.TrimSpace(query)
	var rows []db.SamProduct
	var err error
	switch {
	case len(ids) > 0:
		rows, err = store.SamProductsByRefs(ctx, db.SamProductsByRefsParams{Ids: ids, Query: "", Limit: maxRows + 1})
		if err == nil && len(rows) != len(uniqueInt64(ids)) {
			return nil, huma.Error404NotFound("operator.product_not_found")
		}
	case query != "":
		rows, err = store.SamProductsByRefs(ctx, db.SamProductsByRefsParams{Ids: []int64{}, Query: query, Limit: maxRows + 1})
		if err != nil {
			break
		}
		if len(rows) == 0 {
			return nil, huma.Error404NotFound("operator.product_not_found")
		}
		if !all {
			exact := make([]db.SamProduct, 0, 1)
			for _, row := range rows {
				if strings.EqualFold(row.Name, query) {
					exact = append(exact, row)
				}
			}
			if len(exact) == 1 {
				return exact, nil
			}
			if len(rows) == 1 {
				return rows, nil
			}
			return nil, huma.Error409Conflict("operator.ambiguous_product")
		}
	case all:
		rows, err = store.SamProductsByRefs(ctx, db.SamProductsByRefsParams{Ids: []int64{}, Query: "", Limit: maxRows + 1})
	default:
		return nil, huma.Error422UnprocessableEntity("operator.target_required")
	}
	if err != nil {
		return nil, fmt.Errorf("resolve SAM products: %w", err)
	}
	if len(rows) > maxRows {
		return nil, huma.Error422UnprocessableEntity("operator.limit_exceeded")
	}
	return rows, nil
}

func buildStatusPlan(ctx context.Context, store Store, args setStatusArgs, intent string) (*Plan, error) {
	status := strings.ToLower(strings.TrimSpace(args.Status))
	if status != "active" && status != "inactive" && status != "archived" {
		return nil, huma.Error422UnprocessableEntity("operator.invalid_status")
	}
	products, err := resolveProducts(ctx, store, args.ProductIDs, args.Query, args.All)
	if err != nil {
		return nil, err
	}
	changed := make([]db.SamProduct, 0, len(products))
	for _, product := range products {
		if product.Status != status {
			changed = append(changed, product)
		}
	}
	if len(changed) == 0 {
		return nil, huma.Error422UnprocessableEntity("operator.no_changes")
	}
	if len(changed) > maxRows {
		return nil, huma.Error422UnprocessableEntity("operator.limit_exceeded")
	}
	samples := make([]map[string]any, 0, 20)
	diffs := make([]SamOperatorDiff, 0, 20)
	before := make([]productValue, 0, len(changed))
	after := make([]productValue, 0, len(changed))
	fingerprintRows := make([]map[string]any, 0, len(changed))
	for index, product := range changed {
		value := productValue{ID: product.ID, Name: product.Name, Status: product.Status, Category: product.Category}
		before = append(before, value)
		after = append(after, productValue{ID: product.ID, Name: product.Name, Status: status, Category: product.Category})
		if index < 20 {
			samples = append(samples, map[string]any{"id": product.ID, "name": product.Name, "from": product.Status, "to": status})
			diffs = append(diffs, SamOperatorDiff{Target: fmt.Sprintf("sam_products:%d", product.ID), Field: "status", Old: product.Status, New: status})
		}
		fingerprintRows = append(fingerprintRows, map[string]any{"id": product.ID, "old": product.Status})
	}
	hash, err := fingerprintOf(fingerprintRows)
	if err != nil {
		return nil, err
	}
	requiresPhrase, phrase := phraseFor(args.All)
	warnings := []string{}
	if args.All {
		warnings = append(warnings, "Запрос затрагивает все продукты; для применения нужна контрольная фраза.")
	}
	if len(changed) > 20 {
		warnings = append(warnings, "В preview показаны первые 20 строк.")
	}
	ids := make([]int64, 0, len(changed))
	for _, product := range changed {
		ids = append(ids, product.ID)
	}
	summary := fmt.Sprintf("Перевести продуктов в статус %s: %d.", status, len(changed))
	if intent == "archive_product" {
		summary = fmt.Sprintf("Архивировать продуктов: %d.", len(changed))
	}
	plan := &Plan{
		Intent:         intent,
		Summary:        summary,
		Affected:       len(changed),
		Samples:        samples,
		Diffs:          diffs,
		Warnings:       warnings,
		RequiresPhrase: requiresPhrase,
		Phrase:         phrase,
		Fingerprint:    hash,
	}
	plan.Apply = func(ctx context.Context, store Store) (ExecResult, error) {
		var rows int64
		var err error
		if intent == "archive_product" {
			rows, err = store.SamArchiveProducts(ctx, ids)
		} else {
			rows, err = store.SamUpdateProductStatus(ctx, db.SamUpdateProductStatusParams{Status: status, Ids: ids})
		}
		if err != nil {
			return ExecResult{}, err
		}
		if rows != int64(len(ids)) {
			return ExecResult{}, fmt.Errorf("%w: status update changed %d rows, expected %d", errStale, rows, len(ids))
		}
		return ExecResult{Before: productChange{Products: before}, After: productChange{Products: after}, Affected: len(ids)}, nil
	}
	return plan, nil
}

func buildCategoryPlan(ctx context.Context, store Store, args setCategoryArgs) (*Plan, error) {
	category := strings.TrimSpace(args.Category)
	if category == "" || len([]rune(category)) > 120 {
		return nil, huma.Error422UnprocessableEntity("operator.invalid_category")
	}
	products, err := resolveProducts(ctx, store, args.ProductIDs, args.Query, args.All)
	if err != nil {
		return nil, err
	}
	changed := make([]db.SamProduct, 0, len(products))
	for _, product := range products {
		if product.Category != category {
			changed = append(changed, product)
		}
	}
	if len(changed) == 0 {
		return nil, huma.Error422UnprocessableEntity("operator.no_changes")
	}
	if len(changed) > maxRows {
		return nil, huma.Error422UnprocessableEntity("operator.limit_exceeded")
	}
	samples := make([]map[string]any, 0, 20)
	diffs := make([]SamOperatorDiff, 0, 20)
	before := make([]productValue, 0, len(changed))
	after := make([]productValue, 0, len(changed))
	fingerprintRows := make([]map[string]any, 0, len(changed))
	ids := make([]int64, 0, len(changed))
	for index, product := range changed {
		ids = append(ids, product.ID)
		before = append(before, productValue{ID: product.ID, Name: product.Name, Status: product.Status, Category: product.Category})
		after = append(after, productValue{ID: product.ID, Name: product.Name, Status: product.Status, Category: category})
		if index < 20 {
			samples = append(samples, map[string]any{"id": product.ID, "name": product.Name, "from": product.Category, "to": category})
			diffs = append(diffs, SamOperatorDiff{Target: fmt.Sprintf("sam_products:%d", product.ID), Field: "category", Old: product.Category, New: category})
		}
		fingerprintRows = append(fingerprintRows, map[string]any{"id": product.ID, "old": product.Category})
	}
	hash, err := fingerprintOf(fingerprintRows)
	if err != nil {
		return nil, err
	}
	requiresPhrase, phrase := phraseFor(args.All)
	warnings := []string{}
	if len(changed) > 20 {
		warnings = append(warnings, "В preview показаны первые 20 строк.")
	}
	plan := &Plan{
		Intent:         "set_category",
		Summary:        fmt.Sprintf("Установить категорию «%s» у %d продуктов.", category, len(changed)),
		Affected:       len(changed),
		Samples:        samples,
		Diffs:          diffs,
		Warnings:       warnings,
		RequiresPhrase: requiresPhrase,
		Phrase:         phrase,
		Fingerprint:    hash,
	}
	plan.Apply = func(ctx context.Context, store Store) (ExecResult, error) {
		rows, err := store.SamUpdateProductCategory(ctx, db.SamUpdateProductCategoryParams{Category: category, Ids: ids})
		if err != nil {
			return ExecResult{}, err
		}
		if rows != int64(len(ids)) {
			return ExecResult{}, fmt.Errorf("%w: category update changed %d rows, expected %d", errStale, rows, len(ids))
		}
		return ExecResult{Before: productChange{Products: before}, After: productChange{Products: after}, Affected: len(ids)}, nil
	}
	return plan, nil
}

func productStems(name string) []string {
	normalized := normalizeText(name)
	seen := make(map[string]bool)
	result := make([]string, 0, 6)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if len([]rune(value)) < 3 || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, token := range strings.Fields(normalized) {
		runes := []rune(token)
		if len(runes) < 4 {
			continue
		}
		size := len(runes) - 2
		if size < 4 {
			size = 4
		}
		if size > len(runes) {
			size = len(runes)
		}
		add(string(runes[:size]))
		if strings.HasPrefix(token, "chrom") {
			add("chrom")
		}
	}
	return result
}

func buildMatchAliasesPlan(ctx context.Context, store Store, args matchAliasesArgs) (*Plan, error) {
	product, err := resolveProductSingle(ctx, store, args.Product)
	if err != nil {
		return nil, err
	}
	patterns := make([]string, 0, len(args.Aliases)+4)
	for _, alias := range args.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" {
			patterns = append(patterns, alias)
		}
	}
	if len(patterns) == 0 {
		patterns = productStems(product.Name)
	}
	if len(patterns) == 0 {
		return nil, huma.Error422UnprocessableEntity("operator.aliases_required")
	}
	names, err := store.SamUnmatchedNamesMatching(ctx, db.SamUnmatchedNamesMatchingParams{Patterns: patterns, Limit: 200})
	if err != nil {
		return nil, fmt.Errorf("find unmatched SAM names: %w", err)
	}
	if len(names) == 0 {
		return nil, huma.Error422UnprocessableEntity("operator.no_candidates")
	}
	type aliasPair struct {
		raw        string
		normalized string
	}
	pairs := make([]aliasPair, 0, len(names))
	for _, row := range names {
		pairs = append(pairs, aliasPair{raw: row.RawName, normalized: normalizeAlias(row.RawName)})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].raw < pairs[j].raw })
	rawNames := make([]string, 0, len(pairs))
	normalized := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		rawNames = append(rawNames, pair.raw)
		normalized = append(normalized, pair.normalized)
	}
	conflicts, err := store.SamAliasConflicts(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("check SAM alias conflicts: %w", err)
	}
	conflictProducts := make(map[string]int64, len(conflicts))
	for _, conflict := range conflicts {
		if conflict.ProductID != product.ID {
			conflictProducts[conflict.NormalizedAlias] = conflict.ProductID
		}
	}
	if len(conflictProducts) > 0 {
		return nil, huma.Error409Conflict("operator.alias_conflict")
	}
	existing := make(map[string]bool, len(conflicts))
	for _, conflict := range conflicts {
		existing[conflict.NormalizedAlias] = true
	}
	aliasesToCreate := make([]string, 0, len(rawNames))
	normalizedToCreate := make([]string, 0, len(rawNames))
	for index, name := range rawNames {
		if existing[normalized[index]] {
			continue
		}
		aliasesToCreate = append(aliasesToCreate, name)
		normalizedToCreate = append(normalizedToCreate, normalized[index])
	}
	installations, err := store.SamInstallationsByRawNames(ctx, rawNames)
	if err != nil {
		return nil, fmt.Errorf("load SAM installations by raw name: %w", err)
	}
	filtered := make([]db.SamInstallationsByRawNamesRow, 0, len(installations))
	installIDs := make([]int64, 0, len(installations))
	for _, installation := range installations {
		if installation.ProductID.Valid {
			continue
		}
		filtered = append(filtered, installation)
		installIDs = append(installIDs, installation.ID)
	}
	if len(filtered) == 0 {
		return nil, huma.Error422UnprocessableEntity("operator.no_candidates")
	}
	affected := len(filtered) + len(aliasesToCreate)
	if affected > maxRows {
		return nil, huma.Error422UnprocessableEntity("operator.limit_exceeded")
	}
	sort.Slice(installIDs, func(i, j int) bool { return installIDs[i] < installIDs[j] })
	samples := make([]map[string]any, 0, 20)
	diffs := make([]SamOperatorDiff, 0, 20)
	for index, installation := range filtered {
		if index >= 20 {
			break
		}
		samples = append(samples, map[string]any{"type": "installation", "id": installation.ID, "rawName": installation.RawName, "toProductId": product.ID})
		diffs = append(diffs, SamOperatorDiff{Target: fmt.Sprintf("sam_installations:%d", installation.ID), Field: "product_id", Old: nil, New: product.ID})
	}
	for _, alias := range aliasesToCreate {
		if len(samples) >= 20 {
			break
		}
		samples = append(samples, map[string]any{"type": "alias", "alias": alias, "productId": product.ID})
		diffs = append(diffs, SamOperatorDiff{Target: "sam_product_aliases:new", Field: "alias", Old: nil, New: alias})
	}
	warnings := []string{}
	if len(filtered) > 20 {
		warnings = append(warnings, "В preview показаны первые 20 строк.")
	}
	if len(existing) > 0 {
		warnings = append(warnings, fmt.Sprintf("Алиасы, уже привязанные к продукту: %d.", len(existing)))
	}
	requiresPhrase, phrase := phraseFor(args.All)
	if args.All {
		warnings = append(warnings, "Запрос затрагивает все найденные хромы; для применения нужна контрольная фраза.")
	}
	hash, err := fingerprintOf(map[string]any{
		"productId":  product.ID,
		"installs":   installIDs,
		"rawNames":   rawNames,
		"newAliases": normalizedToCreate,
	})
	if err != nil {
		return nil, err
	}
	plan := &Plan{
		Intent:         "match_aliases",
		Summary:        fmt.Sprintf("Сопоставить %d установок с «%s» и создать %d алиасов.", len(filtered), product.Name, len(aliasesToCreate)),
		Affected:       affected,
		Samples:        samples,
		Diffs:          diffs,
		Warnings:       warnings,
		RequiresPhrase: requiresPhrase,
		Phrase:         phrase,
		Fingerprint:    hash,
	}
	plan.Apply = func(ctx context.Context, store Store) (ExecResult, error) {
		locked, err := store.SamInstallationsByRawNamesForUpdate(ctx, rawNames)
		if err != nil {
			return ExecResult{}, err
		}
		lockedIDs := make(map[int64]bool, len(locked))
		for _, installation := range locked {
			if !installation.ProductID.Valid {
				lockedIDs[installation.ID] = true
			}
		}
		for _, id := range installIDs {
			if !lockedIDs[id] {
				return ExecResult{}, fmt.Errorf("%w: installation %d changed", errStale, id)
			}
		}
		created := make([]aliasValue, 0, len(aliasesToCreate))
		if len(aliasesToCreate) > 0 {
			rows, err := store.SamCreateAliases(ctx, db.SamCreateAliasesParams{
				ProductID:         product.ID,
				CreatedBy:         "sam-operator",
				Aliases:           aliasesToCreate,
				NormalizedAliases: normalizedToCreate,
			})
			if err != nil {
				return ExecResult{}, err
			}
			if len(rows) != len(aliasesToCreate) {
				return ExecResult{}, fmt.Errorf("%w: created %d aliases, expected %d", errStale, len(rows), len(aliasesToCreate))
			}
			for _, row := range rows {
				created = append(created, aliasValue{ID: row.ID, Alias: row.Alias, NormalizedAlias: row.NormalizedAlias})
			}
		}
		rows, err := store.SamSetInstallationProduct(ctx, db.SamSetInstallationProductParams{ProductID: product.ID, Ids: installIDs})
		if err != nil {
			return ExecResult{}, err
		}
		if rows != int64(len(installIDs)) {
			return ExecResult{}, fmt.Errorf("%w: mapped %d installations, expected %d", errStale, rows, len(installIDs))
		}
		beforeInstallations := make([]installationValue, 0, len(filtered))
		afterInstallations := make([]installationValue, 0, len(filtered))
		for _, installation := range filtered {
			beforeInstallations = append(beforeInstallations, installationValue{ID: installation.ID, RawName: installation.RawName})
			productID := product.ID
			afterInstallations = append(afterInstallations, installationValue{ID: installation.ID, RawName: installation.RawName, ProductID: &productID})
		}
		return ExecResult{
			Before:   matchBefore{Aliases: []aliasValue{}, Installations: beforeInstallations},
			After:    matchAfter{Aliases: created, Installations: afterInstallations},
			Affected: len(installIDs) + len(created),
		}, nil
	}
	return plan, nil
}

func buildCreateProductPlan(ctx context.Context, store Store, args createProductArgs) (*Plan, error) {
	name := strings.TrimSpace(args.Name)
	if name == "" || len([]rune(name)) > 200 {
		return nil, huma.Error422UnprocessableEntity("operator.invalid_product")
	}
	status := strings.ToLower(strings.TrimSpace(args.Status))
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "inactive" && status != "archived" {
		return nil, huma.Error422UnprocessableEntity("operator.invalid_status")
	}
	category := strings.TrimSpace(args.Category)
	if len([]rune(category)) > 120 {
		return nil, huma.Error422UnprocessableEntity("operator.invalid_category")
	}
	exists, err := store.SamProductNameExists(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("check SAM product name: %w", err)
	}
	if exists {
		return nil, huma.Error409Conflict("operator.product_exists")
	}
	aliases := make([]string, 0, len(args.Aliases))
	normalized := make([]string, 0, len(args.Aliases))
	for _, alias := range args.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		aliases = append(aliases, alias)
		normalized = append(normalized, normalizeAlias(alias))
	}
	if len(aliases) > 0 {
		conflicts, err := store.SamAliasConflicts(ctx, normalized)
		if err != nil {
			return nil, fmt.Errorf("check SAM alias conflicts: %w", err)
		}
		if len(conflicts) > 0 {
			return nil, huma.Error409Conflict("operator.alias_conflict")
		}
	}
	affected := 1 + len(aliases)
	if affected > maxRows {
		return nil, huma.Error422UnprocessableEntity("operator.limit_exceeded")
	}
	samples := []map[string]any{{"name": name, "vendor": strings.TrimSpace(args.Vendor), "version": strings.TrimSpace(args.Version), "category": category, "status": status}}
	diffs := []SamOperatorDiff{{Target: "sam_products:new", Field: "name", Old: nil, New: name}}
	for _, alias := range aliases {
		if len(diffs) >= 20 {
			break
		}
		diffs = append(diffs, SamOperatorDiff{Target: "sam_product_aliases:new", Field: "alias", Old: nil, New: alias})
	}
	hash, err := fingerprintOf(map[string]any{"name": name, "vendor": args.Vendor, "version": args.Version, "category": category, "status": status, "aliases": normalized})
	if err != nil {
		return nil, err
	}
	requiresPhrase, phrase := phraseFor(args.All)
	plan := &Plan{
		Intent:         "create_product",
		Summary:        fmt.Sprintf("Создать продукт «%s» и %d алиасов.", name, len(aliases)),
		Affected:       affected,
		Samples:        samples,
		Diffs:          diffs,
		Warnings:       []string{},
		RequiresPhrase: requiresPhrase,
		Phrase:         phrase,
		Fingerprint:    hash,
	}
	plan.Apply = func(ctx context.Context, store Store) (ExecResult, error) {
		exists, err := store.SamProductNameExists(ctx, name)
		if err != nil {
			return ExecResult{}, err
		}
		if exists {
			return ExecResult{}, fmt.Errorf("%w: product already exists", errStale)
		}
		product, err := store.SamCreateProduct(ctx, db.SamCreateProductParams{
			Name:     name,
			Vendor:   strings.TrimSpace(args.Vendor),
			Version:  strings.TrimSpace(args.Version),
			Category: category,
			Status:   status,
		})
		if err != nil {
			return ExecResult{}, err
		}
		created := make([]aliasValue, 0, len(aliases))
		if len(aliases) > 0 {
			rows, err := store.SamCreateAliases(ctx, db.SamCreateAliasesParams{
				ProductID:         product.ID,
				CreatedBy:         "sam-operator",
				Aliases:           aliases,
				NormalizedAliases: normalized,
			})
			if err != nil {
				return ExecResult{}, err
			}
			if len(rows) != len(aliases) {
				return ExecResult{}, fmt.Errorf("%w: created %d aliases, expected %d", errStale, len(rows), len(aliases))
			}
			for _, row := range rows {
				created = append(created, aliasValue{ID: row.ID, Alias: row.Alias, NormalizedAlias: row.NormalizedAlias})
			}
		}
		return ExecResult{
			Before: nil,
			After: productCreated{
				Product: productValue{ID: product.ID, Name: product.Name, Status: product.Status, Category: product.Category},
				Aliases: created,
			},
			Affected: 1 + len(created),
		}, nil
	}
	return plan, nil
}

type licenseTarget struct {
	old licenseValue
	new licenseValue
}

func buildLicensePlan(ctx context.Context, store Store, args updateLicenseArgs) (*Plan, error) {
	productIDs := make([]int64, 0, 1)
	if strings.TrimSpace(args.Product) != "" {
		product, err := resolveProductSingle(ctx, store, args.Product)
		if err != nil {
			return nil, err
		}
		productIDs = append(productIDs, product.ID)
	}
	if len(args.LicenseIDs) == 0 && len(productIDs) == 0 {
		return nil, huma.Error422UnprocessableEntity("operator.target_required")
	}
	rows, err := store.SamLicensesByRefs(ctx, db.SamLicensesByRefsParams{Ids: args.LicenseIDs, ProductIds: productIDs})
	if err != nil {
		return nil, fmt.Errorf("load SAM licenses: %w", err)
	}
	if len(rows) == 0 {
		return nil, huma.Error404NotFound("operator.license_not_found")
	}
	if len(rows) > maxRows {
		return nil, huma.Error422UnprocessableEntity("operator.limit_exceeded")
	}
	targets := make([]licenseTarget, 0, len(rows))
	for _, row := range rows {
		desired, err := desiredLicense(row, args)
		if err != nil {
			return nil, err
		}
		old := licenseValueFrom(row)
		if old == desired {
			continue
		}
		targets = append(targets, licenseTarget{old: old, new: desired})
	}
	if len(targets) == 0 {
		return nil, huma.Error422UnprocessableEntity("operator.no_changes")
	}
	samples := make([]map[string]any, 0, 20)
	diffs := make([]SamOperatorDiff, 0, 20)
	before := make([]licenseValue, 0, len(targets))
	after := make([]licenseValue, 0, len(targets))
	fingerprintRows := make([]map[string]any, 0, len(targets))
	for index, target := range targets {
		before = append(before, target.old)
		after = append(after, target.new)
		if index < 20 {
			samples = append(samples, map[string]any{
				"licenseId": target.old.ID, "productId": target.old.ProductID,
				"from": target.old, "to": target.new,
			})
			diffs = append(diffs, SamOperatorDiff{Target: fmt.Sprintf("sam_licenses:%d", target.old.ID), Field: "license", Old: target.old, New: target.new})
		}
		fingerprintRows = append(fingerprintRows, map[string]any{"id": target.old.ID, "old": target.old})
	}
	hash, err := fingerprintOf(fingerprintRows)
	if err != nil {
		return nil, err
	}
	warnings := []string{}
	if len(targets) > 20 {
		warnings = append(warnings, "В preview показаны первые 20 строк.")
	}
	requiresPhrase, phrase := phraseFor(args.All)
	plan := &Plan{
		Intent:         "update_license",
		Summary:        fmt.Sprintf("Обновить лицензий: %d.", len(targets)),
		Affected:       len(targets),
		Samples:        samples,
		Diffs:          diffs,
		Warnings:       warnings,
		RequiresPhrase: requiresPhrase,
		Phrase:         phrase,
		Fingerprint:    hash,
	}
	plan.Apply = func(ctx context.Context, store Store) (ExecResult, error) {
		locked, err := store.SamLicensesForUpdate(ctx, db.SamLicensesForUpdateParams{Ids: args.LicenseIDs, ProductIds: productIDs})
		if err != nil {
			return ExecResult{}, err
		}
		current := make(map[int64]db.SamLicense, len(locked))
		for _, row := range locked {
			current[row.ID] = row
		}
		for _, target := range targets {
			row, ok := current[target.old.ID]
			if !ok || licenseValueFrom(row) != target.old {
				return ExecResult{}, fmt.Errorf("%w: license %d changed", errStale, target.old.ID)
			}
		}
		for _, target := range targets {
			rows, err := store.SamUpdateLicense(ctx, db.SamUpdateLicenseParams{
				ID:          target.new.ID,
				LicenseType: target.new.LicenseType,
				Seats:       target.new.Seats,
				CostPerSeat: target.new.CostPerSeat,
				Currency:    target.new.Currency,
				StartsAt:    dateFromString(target.new.StartsAt),
				ExpiresAt:   dateFromString(target.new.ExpiresAt),
				Status:      target.new.Status,
			})
			if err != nil {
				return ExecResult{}, err
			}
			if rows != 1 {
				return ExecResult{}, fmt.Errorf("%w: license %d was not updated", errStale, target.new.ID)
			}
		}
		return ExecResult{Before: licenseChange{Licenses: before}, After: licenseChange{Licenses: after}, Affected: len(targets)}, nil
	}
	return plan, nil
}

func desiredLicense(row db.SamLicense, args updateLicenseArgs) (licenseValue, error) {
	value := licenseValueFrom(row)
	if args.LicenseType != nil {
		value.LicenseType = strings.ToLower(strings.TrimSpace(*args.LicenseType))
		switch value.LicenseType {
		case "subscription", "perpetual", "open_source", "trial":
		default:
			return licenseValue{}, huma.Error422UnprocessableEntity("operator.invalid_license_type")
		}
	}
	if args.Seats != nil {
		if *args.Seats < 0 {
			return licenseValue{}, huma.Error422UnprocessableEntity("operator.invalid_seats")
		}
		value.Seats = *args.Seats
	}
	if args.CostPerSeat != nil {
		if *args.CostPerSeat < 0 {
			return licenseValue{}, huma.Error422UnprocessableEntity("operator.invalid_cost")
		}
		value.CostPerSeat = *args.CostPerSeat
	}
	if args.Currency != nil {
		value.Currency = strings.ToUpper(strings.TrimSpace(*args.Currency))
		if value.Currency == "" {
			return licenseValue{}, huma.Error422UnprocessableEntity("operator.invalid_currency")
		}
	}
	if args.Status != nil {
		value.Status = strings.ToLower(strings.TrimSpace(*args.Status))
		switch value.Status {
		case "active", "expired", "cancelled":
		default:
			return licenseValue{}, huma.Error422UnprocessableEntity("operator.invalid_status")
		}
	}
	if args.StartsAt != nil {
		date, err := parseDate(*args.StartsAt)
		if err != nil {
			return licenseValue{}, huma.Error422UnprocessableEntity("operator.invalid_date")
		}
		value.StartsAt = date
	}
	if args.ExpiresAt != nil {
		date, err := parseDate(*args.ExpiresAt)
		if err != nil {
			return licenseValue{}, huma.Error422UnprocessableEntity("operator.invalid_date")
		}
		value.ExpiresAt = date
	}
	return value, nil
}

func licenseValueFrom(row db.SamLicense) licenseValue {
	return licenseValue{
		ID:          row.ID,
		ProductID:   row.ProductID,
		LicenseType: row.LicenseType,
		Seats:       row.Seats,
		CostPerSeat: row.CostPerSeat,
		Currency:    row.Currency,
		StartsAt:    dateString(row.StartsAt),
		ExpiresAt:   dateString(row.ExpiresAt),
		Status:      row.Status,
	}
}

func parseDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", err
	}
	return parsed.Format("2006-01-02"), nil
}

func dateString(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func dateFromString(value string) pgtype.Date {
	if value == "" {
		return pgtype.Date{}
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: parsed, Valid: true}
}

func normalizeText(value string) string {
	replacer := strings.NewReplacer("ё", "е", "Ё", "Е", ",", " ", ".", " ", "-", " ", "_", " ")
	return strings.ToLower(strings.Join(strings.Fields(replacer.Replace(value)), " "))
}

func normalizeAlias(value string) string {
	return normalizeText(value)
}

func uniqueInt64(values []int64) []int64 {
	seen := make(map[int64]bool, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
