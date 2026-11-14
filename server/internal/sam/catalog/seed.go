// Package catalog turns demo/sam/catalog.json into a normalized seed for the
// SAM catalog. It deliberately does not open a database: the SAM tables are
// created by the migration owned by the schema task, so this package only
// validates, maps and deduplicates vendor, product and license records.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"hackathon/internal/sam/importer"
)

// Vendor is a deduplicated vendor with both display and comparison names.
type Vendor struct {
	Name       string `json:"name"`
	Normalized string `json:"normalized"`
}

// Product is a catalog entry ready for the seed. Aliases are normalized and
// sorted so the same catalog.json always produces the same seed.
type Product struct {
	Vendor           string   `json:"vendor"`
	VendorNormalized string   `json:"vendorNormalized"`
	Name             string   `json:"name"`
	Normalized       string   `json:"normalized"`
	Aliases          []string `json:"aliases,omitempty"`
	Category         string   `json:"category,omitempty"`
}

// License is an optional entitlement attached to a product. Missing values are
// preserved as empty; the catalog loader does not invent commercial terms.
type License struct {
	Product  string `json:"product"`
	Vendor   string `json:"vendor"`
	Metric   string `json:"metric,omitempty"`
	Quantity int    `json:"quantity,omitempty"`
	Expires  string `json:"expiresAt,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

// Seed is deterministic output for one catalog.json.
type Seed struct {
	Vendors  []Vendor  `json:"vendors"`
	Products []Product `json:"products"`
	Licenses []License `json:"licenses"`
}

// Issue is a non-fatal catalog problem. The seed still contains every valid
// record so an operator can review the file before writing it.
type Issue struct {
	Index   int    `json:"index"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LoadResult carries the normalized seed and recoverable problems.
type LoadResult struct {
	Seed   Seed
	Issues []Issue
}

// Load reads a catalog document. It accepts either a top-level array of
// products or an object with products, vendors and licenses arrays.
func Load(r io.Reader) (LoadResult, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return LoadResult{}, fmt.Errorf("decode catalog: %w", err)
	}

	productNodes, vendorNodes, licenseNodes, err := documentSections(document)
	if err != nil {
		return LoadResult{}, err
	}
	if len(productNodes) == 0 {
		return LoadResult{}, ErrEmptyCatalog
	}

	builder := newSeedBuilder()
	for index, node := range vendorNodes {
		name, ok := nodeName(node)
		if !ok {
			builder.issue(index, "vendor", "catalog.vendor_invalid", "vendor must be a name or an object with a name")
			continue
		}
		builder.addVendor(name)
	}
	for index, node := range productNodes {
		object, ok := node.(map[string]any)
		if !ok {
			builder.issue(index, "product", "catalog.product_invalid", "product must be an object")
			continue
		}
		builder.addProduct(index, object)
	}
	for index, node := range licenseNodes {
		object, ok := node.(map[string]any)
		if !ok {
			builder.issue(index, "license", "catalog.license_invalid", "license must be an object")
			continue
		}
		builder.addLicense(index, object, "")
	}

	return builder.result(), nil
}

// LoadFile is the entry point for demo/sam/catalog.json. The path is supplied
// by the caller so the repository does not depend on one machine's layout.
func LoadFile(ctx context.Context, path string) (LoadResult, error) {
	if err := ctx.Err(); err != nil {
		return LoadResult{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return LoadResult{}, fmt.Errorf("open catalog %s: %w", path, err)
	}
	defer file.Close()
	return Load(file)
}

// ErrEmptyCatalog means the document was valid JSON but contained no products.
var ErrEmptyCatalog = errors.New("catalog: no products")

type seedBuilder struct {
	vendors    []Vendor
	vendorKeys map[string]struct{}
	products   []Product
	productIdx map[string]int
	licenses   []License
	licenseKey map[string]struct{}
	issues     []Issue
}

func newSeedBuilder() *seedBuilder {
	return &seedBuilder{
		vendorKeys: make(map[string]struct{}),
		productIdx: make(map[string]int),
		licenseKey: make(map[string]struct{}),
	}
}

func (b *seedBuilder) result() LoadResult {
	sort.Slice(b.vendors, func(i, j int) bool { return b.vendors[i].Normalized < b.vendors[j].Normalized })
	sort.Slice(b.products, func(i, j int) bool {
		if b.products[i].VendorNormalized == b.products[j].VendorNormalized {
			return b.products[i].Normalized < b.products[j].Normalized
		}
		return b.products[i].VendorNormalized < b.products[j].VendorNormalized
	})
	for index := range b.products {
		sort.Strings(b.products[index].Aliases)
	}
	sort.Slice(b.licenses, func(i, j int) bool {
		if b.licenses[i].Product == b.licenses[j].Product {
			return b.licenses[i].Metric < b.licenses[j].Metric
		}
		return b.licenses[i].Product < b.licenses[j].Product
	})
	return LoadResult{Seed: Seed{Vendors: b.vendors, Products: b.products, Licenses: b.licenses}, Issues: b.issues}
}

func (b *seedBuilder) issue(index int, field, code, message string) {
	b.issues = append(b.issues, Issue{Index: index, Field: field, Code: code, Message: message})
}

func (b *seedBuilder) addVendor(name string) {
	display := strings.TrimSpace(name)
	normalized := importer.NormalizeVendor(display)
	if normalized == "" {
		return
	}
	if _, exists := b.vendorKeys[normalized]; exists {
		return
	}
	b.vendorKeys[normalized] = struct{}{}
	b.vendors = append(b.vendors, Vendor{Name: display, Normalized: normalized})
}

func (b *seedBuilder) addProduct(index int, object map[string]any) {
	name := stringField(object, "name", "product", "productName", "title", "software", "displayName")
	vendor := stringField(object, "vendor", "publisher", "manufacturer", "company")
	if name == "" {
		b.issue(index, "name", "catalog.product_without_name", "product name is required")
		return
	}
	if vendor == "" {
		b.issue(index, "vendor", "catalog.product_without_vendor", "product vendor is required")
		return
	}

	normalizedName := importer.NormalizeName(name)
	normalizedVendor := importer.NormalizeVendor(vendor)
	if normalizedName == "" {
		b.issue(index, "name", "catalog.product_name_empty_after_normalization", "product name is empty after normalization")
		return
	}
	if normalizedVendor == "" {
		b.issue(index, "vendor", "catalog.vendor_empty_after_normalization", "vendor is empty after normalization")
		return
	}

	b.addVendor(vendor)
	key := normalizedVendor + "\x1f" + normalizedName
	aliases := stringList(object, "aliases", "alias", "aka", "synonyms")
	if existingIndex, exists := b.productIdx[key]; exists {
		b.mergeAliases(existingIndex, aliases, normalizedName)
		b.issue(index, "product", "catalog.duplicate_product", "product duplicates "+b.products[existingIndex].Name)
	} else {
		product := Product{
			Vendor:           strings.TrimSpace(vendor),
			VendorNormalized: normalizedVendor,
			Name:             strings.TrimSpace(name),
			Normalized:       normalizedName,
			Category:         stringField(object, "category", "type", "kind"),
		}
		b.productIdx[key] = len(b.products)
		b.products = append(b.products, product)
		b.mergeAliases(len(b.products)-1, aliases, normalizedName)
	}

	for _, licenseNode := range objectList(object, "licenses", "license", "licensing", "entitlements") {
		licenseObject, ok := licenseNode.(map[string]any)
		if !ok {
			b.issue(index, "license", "catalog.license_invalid", "license must be an object")
			continue
		}
		b.addLicense(index, licenseObject, key)
	}
}

func (b *seedBuilder) mergeAliases(productIndex int, aliases []string, productName string) {
	seen := make(map[string]struct{}, len(b.products[productIndex].Aliases)+len(aliases))
	for _, alias := range b.products[productIndex].Aliases {
		seen[alias] = struct{}{}
	}
	for _, alias := range aliases {
		normalized := importer.NormalizeName(alias)
		if normalized == "" || normalized == productName {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		b.products[productIndex].Aliases = append(b.products[productIndex].Aliases, normalized)
	}
}

func (b *seedBuilder) addLicense(index int, object map[string]any, productKey string) {
	product := stringField(object, "product", "productName", "name", "software")
	vendor := stringField(object, "vendor", "publisher", "manufacturer")
	if productKey == "" {
		if product == "" || vendor == "" {
			b.issue(index, "product", "catalog.license_without_product", "license product and vendor are required")
			return
		}
		productKey = importer.NormalizeVendor(vendor) + "\x1f" + importer.NormalizeName(product)
	}
	productIndex, exists := b.productIdx[productKey]
	if !exists {
		b.issue(index, "product", "catalog.license_unknown_product", "license references a product that is not in the catalog")
		return
	}
	entry := b.products[productIndex]
	license := License{
		Product: entry.Name,
		Vendor:  entry.Vendor,
		Metric:  stringField(object, "metric", "unit", "licenseMetric", "licenseType"),
		Expires: stringField(object, "expiresAt", "expires", "expiry", "validUntil", "validUntilDate"),
		Notes:   stringField(object, "notes", "note", "description", "comment"),
	}
	if quantity, ok := intField(object, "quantity", "seats", "count", "qty", "amount"); ok {
		license.Quantity = quantity
	}
	if license.Metric == "" && license.Quantity == 0 && license.Expires == "" && license.Notes == "" {
		return
	}
	key := productKey + "\x1f" + license.Metric + "\x1f" + strconv.Itoa(license.Quantity) + "\x1f" + license.Expires
	if _, duplicate := b.licenseKey[key]; duplicate {
		return
	}
	b.licenseKey[key] = struct{}{}
	b.licenses = append(b.licenses, license)
}

func documentSections(document any) (products, vendors, licenses []any, err error) {
	switch typed := document.(type) {
	case []any:
		return typed, nil, nil, nil
	case map[string]any:
		products = objectList(typed, "products", "software", "apps", "catalog")
		vendors = objectList(typed, "vendors", "publishers", "manufacturers")
		licenses = objectList(typed, "licenses", "entitlements")
		return products, vendors, licenses, nil
	default:
		return nil, nil, nil, ErrInvalidDocument
	}
}

// ErrInvalidDocument means the top level is neither a product array nor an
// object with product, vendor and license sections.
var ErrInvalidDocument = errors.New("catalog: top level must be an array or object")

func nodeName(node any) (string, bool) {
	switch typed := node.(type) {
	case string:
		return typed, strings.TrimSpace(typed) != ""
	case map[string]any:
		name := stringField(typed, "name", "vendor", "publisher", "manufacturer")
		return name, name != ""
	default:
		return "", false
	}
}

func objectList(object map[string]any, keys ...string) []any {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case []any:
			return typed
		case map[string]any:
			return []any{typed}
		}
	}
	return nil
}

func stringList(object map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case []any:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					result = append(result, strings.TrimSpace(text))
				}
			}
			return result
		case string:
			result := make([]string, 0)
			for _, item := range strings.Split(typed, ",") {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			return result
		}
	}
	return nil
}

func stringField(object map[string]any, keys ...string) string {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case json.Number:
			return typed.String()
		}
	}
	return ""
}

func intField(object map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case json.Number:
			number, err := typed.Int64()
			if err == nil && number >= 0 {
				return int(number), true
			}
		case float64:
			if typed >= 0 {
				return int(typed), true
			}
		case string:
			number, err := strconv.Atoi(strings.TrimSpace(typed))
			if err == nil && number >= 0 {
				return number, true
			}
		}
	}
	return 0, false
}
