// Command sam-demo-seed loads the anonymized demo pool prepared for the SAM
// case into the production schema. It is idempotent: without -force it does
// nothing when the catalog already has products, and with -force it clears the
// SAM fact tables first. Source data lives in demo/sam and is derived from the
// real inventory by scripts/sam/prepare-real.ts.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"hackathon/internal/sam/catalog"
	"hackathon/internal/sam/importer"
)

type licenseDocument struct {
	Licenses []struct {
		ProductName string `json:"product_name"`
		Metric      string `json:"metric"`
		SeatsTotal  int64  `json:"seats_total"`
		UnitPrice   int64  `json:"unit_price_rub"`
		Currency    string `json:"currency"`
		PurchasedAt string `json:"purchased_at"`
	} `json:"licenses"`
}

type usageDocument struct {
	Usage []struct {
		DeviceID   string `json:"device_id"`
		ProductID  string `json:"product_id"`
		LastUsedAt string `json:"last_used_at"`
	} `json:"usage"`
}

type catalogDocument struct {
	Products []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"products"`
}

func main() {
	dsn := flag.String("dsn", os.Getenv("SAM_DEMO_DSN"), "PostgreSQL DSN")
	dir := flag.String("dir", "demo/sam", "directory with the demo pool")
	force := flag.Bool("force", false, "clear SAM fact tables before seeding")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "sam-demo-seed: -dsn or SAM_DEMO_DSN is required")
		os.Exit(2)
	}
	ctx := context.Background()
	if err := run(ctx, *dsn, *dir, *force); err != nil {
		slog.ErrorContext(ctx, "sam demo seed failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dsn, dir string, force bool) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	var products int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM sam_products").Scan(&products); err != nil {
		return fmt.Errorf("count sam_products: %w", err)
	}
	if products > 0 && !force {
		slog.InfoContext(ctx, "sam demo data already present, skipping", "products", products)
		return nil
	}

	catalogSeed, err := catalog.LoadFile(ctx, filepath.Join(dir, "catalog.json"))
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	statuses, err := loadCatalogStatuses(filepath.Join(dir, "catalog.json"))
	if err != nil {
		return fmt.Errorf("load catalog statuses: %w", err)
	}
	licenses, err := loadLicenses(filepath.Join(dir, "licenses.json"))
	if err != nil {
		return fmt.Errorf("load licenses: %w", err)
	}
	usage, err := loadUsage(filepath.Join(dir, "usage.json"))
	if err != nil {
		return fmt.Errorf("load usage: %w", err)
	}
	rows, err := loadInventory(filepath.Join(dir, "installed_software.csv"))
	if err != nil {
		return fmt.Errorf("load inventory: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if force {
		if _, err := tx.Exec(ctx, "TRUNCATE sam_installations, sam_licenses, sam_product_aliases, sam_products RESTART IDENTITY CASCADE"); err != nil {
			return fmt.Errorf("truncate sam tables: %w", err)
		}
	}

	ids := make(map[string]int64, len(catalogSeed.Products))
	for _, product := range catalogSeed.Products {
		status := statuses[strings.ToLower(product.Name)]
		if status == "" {
			status = "active"
		}
		var id int64
		err := tx.QueryRow(ctx,
			`INSERT INTO sam_products(name, vendor, category, status)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (lower(name)) DO UPDATE
			   SET vendor = EXCLUDED.vendor, category = EXCLUDED.category, status = EXCLUDED.status
			 RETURNING id`,
			product.Name, product.Vendor, product.Category, status,
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert product %q: %w", product.Name, err)
		}
		ids[strings.ToLower(product.Name)] = id
	}

	aliasCount := 0
	for _, product := range catalogSeed.Products {
		productID := ids[strings.ToLower(product.Name)]
		aliases := append([]string{product.Name}, product.Aliases...)
		for _, alias := range aliases {
			normalized := importer.NormalizeName(alias)
			if normalized == "" {
				continue
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO sam_product_aliases(product_id, alias, normalized_alias, created_by)
				 VALUES ($1, $2, $3, 'demo')
				 ON CONFLICT (normalized_alias) DO NOTHING`,
				productID, alias, normalized,
			); err != nil {
				return fmt.Errorf("insert alias %q: %w", alias, err)
			}
			aliasCount++
		}
	}

	licenseCount := 0
	for _, license := range licenses.Licenses {
		productID, ok := ids[strings.ToLower(license.ProductName)]
		if !ok {
			slog.WarnContext(ctx, "license references unknown product", "product", license.ProductName)
			continue
		}
		var starts any
		if license.PurchasedAt != "" {
			if parsed, err := time.Parse("2006-01-02", license.PurchasedAt); err == nil {
				starts = parsed
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO sam_licenses(product_id, license_type, seats, cost_per_seat, currency, starts_at, expires_at, status)
			 VALUES ($1, 'subscription', $2, $3, $4, $5, NULL, 'active')`,
			productID, license.SeatsTotal, license.UnitPrice, license.Currency, starts,
		); err != nil {
			return fmt.Errorf("insert license for %q: %w", license.ProductName, err)
		}
		licenseCount++
	}

	now := time.Now()
	inserted := 0
	for _, row := range rows {
		var productID any
		if row.ProductID != "" {
			if id, ok := ids[strings.ToLower(row.ProductID)]; ok {
				productID = id
			}
		}
		lastSeen := now
		if used, ok := usage[strings.ToLower(row.Device)+"\x00"+strings.ToLower(row.ProductName)]; ok {
			if parsed, err := time.Parse("2006-01-02", used); err == nil {
				lastSeen = parsed
			}
		}
		var installedAt any
		if row.InstallDate != "" {
			if parsed, err := time.Parse("2006-01-02", row.InstallDate); err == nil {
				installedAt = parsed
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO sam_installations(product_id, raw_name, device_name, user_name, version, source, status, installed_at, last_seen_at)
			 VALUES ($1, $2, $3, '', $4, 'demo', 'installed', $5, $6)`,
			productID, row.ProductName, row.Device, row.Version, installedAt, lastSeen,
		); err != nil {
			return fmt.Errorf("insert installation %q on %q: %w", row.ProductName, row.Device, err)
		}
		inserted++
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	slog.InfoContext(ctx, "sam demo data loaded",
		"products", len(catalogSeed.Products), "aliases", aliasCount,
		"licenses", licenseCount, "installations", inserted,
		"catalog_issues", len(catalogSeed.Issues))
	return nil
}

type inventoryRow struct {
	Device      string
	ProductID   string
	ProductName string
	Version     string
	InstallDate string
}

func loadInventory(path string) ([]inventoryRow, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	index := make(map[string]int, len(header))
	for position, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = position
	}
	required := []string{"device", "product_id", "product_name", "version", "install_date", "is_trackable", "is_duplicate"}
	for _, name := range required {
		if _, ok := index[name]; !ok {
			return nil, fmt.Errorf("column %q not found", name)
		}
	}
	var rows []inventoryRow
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		if !flagBool(record, index, "is_trackable") || flagBool(record, index, "is_duplicate") {
			continue
		}
		rows = append(rows, inventoryRow{
			Device:      value(record, index, "device"),
			ProductID:   value(record, index, "product_id"),
			ProductName: value(record, index, "product_name"),
			Version:     value(record, index, "version"),
			InstallDate: value(record, index, "install_date"),
		})
	}
	return rows, nil
}

func value(record []string, index map[string]int, name string) string {
	position, ok := index[name]
	if !ok || position >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[position])
}

func flagBool(record []string, index map[string]int, name string) bool {
	value := value(record, index, name)
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return strings.EqualFold(value, "true") || value == "1"
	}
	return parsed
}

func loadCatalogStatuses(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var document catalogDocument
	if err := json.NewDecoder(file).Decode(&document); err != nil {
		return nil, err
	}
	statuses := make(map[string]string, len(document.Products))
	for _, product := range document.Products {
		status := strings.ToLower(strings.TrimSpace(product.Status))
		if status == "" {
			status = "active"
		}
		statuses[strings.ToLower(product.Name)] = status
	}
	return statuses, nil
}

func loadLicenses(path string) (licenseDocument, error) {
	var document licenseDocument
	file, err := os.Open(path)
	if err != nil {
		return document, err
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(&document); err != nil {
		return document, err
	}
	return document, nil
}

func loadUsage(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var document usageDocument
	if err := json.NewDecoder(file).Decode(&document); err != nil {
		return nil, err
	}
	usage := make(map[string]string, len(document.Usage))
	for _, item := range document.Usage {
		usage[strings.ToLower(item.DeviceID)+"\x00"+strings.ToLower(item.ProductID)] = item.LastUsedAt
	}
	return usage, nil
}

