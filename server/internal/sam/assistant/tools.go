package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/llm"
)

type searchProductsArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type listProductsArgs struct {
	Status string `json:"status"`
	Limit  int    `json:"limit"`
}

type productBalanceArgs struct {
	Product string `json:"product"`
	Limit   int    `json:"limit"`
}

type findInstallationsArgs struct {
	Product   string `json:"product"`
	Query     string `json:"query"`
	Unmatched bool   `json:"unmatched"`
	Limit     int    `json:"limit"`
}

type limitArgs struct {
	Limit int `json:"limit"`
}

type productRow struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	Version  string `json:"version"`
	Category string `json:"category"`
	Status   string `json:"status"`
}

type balanceRow struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Vendor         string `json:"vendor"`
	Status         string `json:"status"`
	LicensedSeats  int64  `json:"licensedSeats"`
	ExpiredSeats   int64  `json:"expiredSeats"`
	Installed      int64  `json:"installed"`
	AvailableSeats int64  `json:"availableSeats"`
	NearestExpiry  string `json:"nearestExpiry,omitempty"`
}

type installationRow struct {
	ID          int64  `json:"id"`
	RawName     string `json:"rawName"`
	ProductName string `json:"productName"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	LastSeen    string `json:"lastSeen"`
}

type licenseRow struct {
	LicenseType string `json:"licenseType"`
	Licenses    int64  `json:"licenses"`
	Seats       int64  `json:"seats"`
	Expired     int64  `json:"expired"`
	Products    int64  `json:"products"`
	AnnualCost  int64  `json:"annualCost"`
	Currency    string `json:"currency"`
}

type riskRow struct {
	Risk     string `json:"risk"`
	Severity string `json:"severity"`
	Affected int64  `json:"affected"`
	Detail   string `json:"detail"`
}

type unmatchedRow struct {
	RawName       string `json:"rawName"`
	Installations int64  `json:"installations"`
	Devices       int64  `json:"devices"`
	LastSeen      string `json:"lastSeen,omitempty"`
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func intSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": 1, "maximum": 50}
}

func tool(name, description string, properties map[string]any, required []string) llm.Tool {
	parameters := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}
	return llm.Tool{Type: "function", Function: llm.Function{Name: name, Description: description, Parameters: parameters}}
}

func toolDefinitions() []llm.Tool {
	return []llm.Tool{
		tool("search_products", "Найти продукты SAM по названию, вендору или алиасу. Возвращает только существующие записи sam_products.", map[string]any{
			"query": stringSchema("Строка поиска, например Google Chrome или Microsoft."),
			"limit": intSchema("Сколько строк вернуть, по умолчанию 10."),
		}, []string{"query"}),
		tool("list_products", "Список продуктов SAM с необязательным фильтром по статусу active, outdated или archived.", map[string]any{
			"status": stringSchema("Пустая строка — все статусы, иначе active, outdated или archived."),
			"limit":  intSchema("Сколько строк вернуть, по умолчанию 20."),
		}, nil),
		tool("product_balance", "Баланс мест по продукту: оплаченные места, установки, свободные места и ближайшее истечение лицензии.", map[string]any{
			"product": stringSchema("Название продукта или его часть."),
			"limit":   intSchema("Сколько совпадений вернуть, по умолчанию 5."),
		}, []string{"product"}),
		tool("find_installations", "Обезличенные установки по продукту или имени. Флаг unmatched=true оставляет только несопоставленные установки.", map[string]any{
			"product":   stringSchema("Название продукта, необязательно."),
			"query":     stringSchema("Часть исходного имени установки, необязательно."),
			"unmatched": map[string]any{"type": "boolean", "description": "Только установки без product_id."},
			"limit":     intSchema("Сколько строк вернуть, по умолчанию 20."),
		}, nil),
		tool("license_summary", "Сводка действующих лицензий по типу: количество лицензий, мест, истёкших и продуктах, годовая стоимость.", map[string]any{}, nil),
		tool("top_risks", "Главные риски SAM: несопоставленные установки, истёкшие лицензии, перерасход мест, неактивные продукты без лицензии.", map[string]any{
			"limit": intSchema("Сколько рисков вернуть, по умолчанию 10."),
		}, nil),
		tool("unmatched_names", "Несопоставленные исходные имена установок с количеством установок и устройств.", map[string]any{
			"limit": intSchema("Сколько имён вернуть, по умолчанию 20."),
		}, nil),
	}
}

func decodeArgs(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("decode tool arguments: %w", err)
	}
	return nil
}

func (s *Service) executeTool(ctx context.Context, name, raw string) (toolEnvelope, error) {
	switch name {
	case "search_products":
		var args searchProductsArgs
		if err := decodeArgs(raw, &args); err != nil {
			return toolEnvelope{}, err
		}
		rows, err := s.store.SearchSamProducts(ctx, dbSearchProductsParams(args.Query, clampLimit(args.Limit, 10, maxToolRows)))
		if err != nil {
			return toolEnvelope{}, fmt.Errorf("search SAM products: %w", err)
		}
		items := make([]productRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, productRow{ID: row.ID, Name: row.Name, Vendor: row.Vendor, Version: row.Version, Category: row.Category, Status: row.Status})
		}
		return toolEnvelope{Source: "sam_products/sam_product_aliases (search_products)", Count: len(items), Data: items}, nil
	case "list_products":
		var args listProductsArgs
		if err := decodeArgs(raw, &args); err != nil {
			return toolEnvelope{}, err
		}
		status := strings.ToLower(strings.TrimSpace(args.Status))
		if status != "" && status != "active" && status != "outdated" && status != "archived" && status != "banned" {
			return toolEnvelope{}, fmt.Errorf("status must be active, outdated, archived or banned")
		}
		rows, err := s.store.ListSamProducts(ctx, dbListProductsParams(status, clampLimit(args.Limit, 20, maxToolRows)))
		if err != nil {
			return toolEnvelope{}, fmt.Errorf("list SAM products: %w", err)
		}
		items := make([]productRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, productRow{ID: row.ID, Name: row.Name, Vendor: row.Vendor, Version: row.Version, Category: row.Category, Status: row.Status})
		}
		return toolEnvelope{Source: "sam_products (list_products)", Count: len(items), Data: items}, nil
	case "product_balance":
		var args productBalanceArgs
		if err := decodeArgs(raw, &args); err != nil {
			return toolEnvelope{}, err
		}
		query := strings.TrimSpace(args.Product)
		if query == "" {
			return toolEnvelope{}, fmt.Errorf("product is required")
		}
		rows, err := s.store.SamProductBalances(ctx, dbProductBalancesParams(query, clampLimit(args.Limit, 5, 20)))
		if err != nil {
			return toolEnvelope{}, fmt.Errorf("load SAM product balances: %w", err)
		}
		items := make([]balanceRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, balanceRow{ID: row.ID, Name: row.Name, Vendor: row.Vendor, Status: row.Status, LicensedSeats: row.LicensedSeats, ExpiredSeats: row.ExpiredSeats, Installed: row.Installed, AvailableSeats: row.AvailableSeats, NearestExpiry: valueString(row.NearestExpiry)})
		}
		return toolEnvelope{Source: "sam_products/sam_licenses/sam_installations (product_balance)", Count: len(items), Data: items}, nil
	case "find_installations":
		var args findInstallationsArgs
		if err := decodeArgs(raw, &args); err != nil {
			return toolEnvelope{}, err
		}
		productID := int64(0)
		if query := strings.TrimSpace(args.Product); query != "" {
			products, err := s.store.SearchSamProducts(ctx, dbSearchProductsParams(query, 10))
			if err != nil {
				return toolEnvelope{}, fmt.Errorf("resolve SAM product: %w", err)
			}
			if len(products) == 0 {
				return toolEnvelope{}, fmt.Errorf("product %q was not found", query)
			}
			productID = products[0].ID
		}
		rows, err := s.store.SamFindInstallations(ctx, dbFindInstallationsParams(args.Unmatched, productID, strings.TrimSpace(args.Query), clampLimit(args.Limit, 20, maxToolRows)))
		if err != nil {
			return toolEnvelope{}, fmt.Errorf("find SAM installations: %w", err)
		}
		items := make([]installationRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, installationRow{ID: row.ID, RawName: row.RawName, ProductName: row.ProductName, Version: row.Version, Status: row.Status, LastSeen: valueString(row.LastSeenAt)})
		}
		return toolEnvelope{Source: "sam_installations (find_installations)", Count: len(items), Data: items}, nil
	case "license_summary":
		rows, err := s.store.SamLicenseSummary(ctx)
		if err != nil {
			return toolEnvelope{}, fmt.Errorf("load SAM license summary: %w", err)
		}
		items := make([]licenseRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, licenseRow{LicenseType: row.LicenseType, Licenses: row.Licenses, Seats: row.Seats, Expired: row.Expired, Products: row.Products, AnnualCost: row.AnnualCost, Currency: "RUB"})
		}
		return toolEnvelope{Source: "sam_licenses (license_summary)", Count: len(items), Data: items}, nil
	case "top_risks":
		var args limitArgs
		if err := decodeArgs(raw, &args); err != nil {
			return toolEnvelope{}, err
		}
		rows, err := s.store.SamTopRisks(ctx, clampLimit(args.Limit, 10, 20))
		if err != nil {
			return toolEnvelope{}, fmt.Errorf("load SAM top risks: %w", err)
		}
		items := make([]riskRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, riskRow{Risk: row.Risk, Severity: row.Severity, Affected: row.Affected, Detail: row.Detail})
		}
		return toolEnvelope{Source: "sam_risk_overview (top_risks)", Count: len(items), Data: items}, nil
	case "unmatched_names":
		var args limitArgs
		if err := decodeArgs(raw, &args); err != nil {
			return toolEnvelope{}, err
		}
		rows, err := s.store.SamUnmatchedNames(ctx, clampLimit(args.Limit, 20, maxToolRows))
		if err != nil {
			return toolEnvelope{}, fmt.Errorf("load SAM unmatched names: %w", err)
		}
		items := make([]unmatchedRow, 0, len(rows))
		for _, row := range rows {
			items = append(items, unmatchedRow{RawName: row.RawName, Installations: row.Installations, Devices: row.Devices, LastSeen: valueString(row.LastSeen)})
		}
		return toolEnvelope{Source: "sam_installations (unmatched_names)", Count: len(items), Data: items}, nil
	default:
		return toolEnvelope{}, fmt.Errorf("tool %q is not allowed", name)
	}
}

func dbSearchProductsParams(query string, limit int32) db.SearchSamProductsParams {
	return db.SearchSamProductsParams{Query: strings.TrimSpace(query), Limit: limit}
}

func dbListProductsParams(status string, limit int32) db.ListSamProductsParams {
	return db.ListSamProductsParams{Status: status, Limit: limit}
}

func dbProductBalancesParams(query string, limit int32) db.SamProductBalancesParams {
	return db.SamProductBalancesParams{Query: strings.TrimSpace(query), Limit: limit}
}

func dbFindInstallationsParams(unmatched bool, productID int64, query string, limit int32) db.SamFindInstallationsParams {
	return db.SamFindInstallationsParams{Unmatched: unmatched, ProductID: productID, Query: query, Limit: limit}
}

func valueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case pgtype.Date:
		if !typed.Valid {
			return ""
		}
		return typed.Time.Format("2006-01-02")
	case pgtype.Timestamptz:
		if !typed.Valid {
			return ""
		}
		return typed.Time.UTC().Format("2006-01-02T15:04:05Z")
	case time.Time:
		return typed.UTC().Format("2006-01-02T15:04:05Z")
	case pgtype.Text:
		if !typed.Valid {
			return ""
		}
		return typed.String
	case interface{ String() string }:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}
