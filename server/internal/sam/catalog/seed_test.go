package catalog

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLoadFixture(t *testing.T) {
	result, err := LoadFile(context.Background(), "testdata/catalog.json")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(result.Seed.Vendors) != 2 {
		t.Fatalf("vendors = %d, want 2: %+v", len(result.Seed.Vendors), result.Seed.Vendors)
	}
	if len(result.Seed.Products) != 2 {
		t.Fatalf("products = %d, want 2: %+v", len(result.Seed.Products), result.Seed.Products)
	}
	if len(result.Seed.Licenses) != 2 {
		t.Fatalf("licenses = %d, want 2: %+v", len(result.Seed.Licenses), result.Seed.Licenses)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != "catalog.duplicate_product" {
		t.Fatalf("issues = %+v, want one duplicate product issue", result.Issues)
	}

	windows := findProduct(t, result.Seed.Products, "Windows 11 Pro")
	if windows.VendorNormalized != "microsoft" || windows.Normalized != "windows 11 pro" {
		t.Fatalf("unexpected Windows product: %+v", windows)
	}
	if !contains(windows.Aliases, "windows 11 professional") {
		t.Fatalf("duplicate alias was lost: %+v", windows.Aliases)
	}
	if !contains(windows.Aliases, "win11 pro") {
		t.Fatalf("aliases were not merged: %+v", windows.Aliases)
	}

	chrome := findProduct(t, result.Seed.Products, "Google Chrome")
	if !contains(chrome.Aliases, "chrome") || !contains(chrome.Aliases, "google chrome browser") {
		t.Fatalf("comma-separated aliases not mapped: %+v", chrome.Aliases)
	}

	license := findLicense(t, result.Seed.Licenses, "Windows 11 Pro")
	if license.Quantity != 50 || license.Metric != "per_device" || license.Expires != "2027-12-31" {
		t.Fatalf("unexpected license: %+v", license)
	}
}

func TestLoadTopLevelArray(t *testing.T) {
	result, err := Load(strings.NewReader(`[{"name":"7-Zip","publisher":"Igor Pavlov","aliases":["7zip"]}]`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(result.Seed.Products) != 1 || len(result.Seed.Vendors) != 1 {
		t.Fatalf("unexpected seed: %+v", result.Seed)
	}
	product := result.Seed.Products[0]
	if product.VendorNormalized != "igor pavlov" || product.Normalized != "7 zip" {
		t.Fatalf("unexpected product: %+v", product)
	}
	if !contains(product.Aliases, "7zip") {
		t.Fatalf("alias missing: %+v", product.Aliases)
	}
}

func TestLoadRejectsInvalidDocuments(t *testing.T) {
	if _, err := Load(strings.NewReader(`{"products":[]}`)); !errors.Is(err, ErrEmptyCatalog) {
		t.Fatalf("empty catalog error = %v, want ErrEmptyCatalog", err)
	}
	if _, err := Load(strings.NewReader(`"text"`)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("invalid document error = %v, want ErrInvalidDocument", err)
	}
}

func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile(context.Background(), "testdata/missing.json")
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}

func findProduct(t *testing.T, products []Product, name string) Product {
	t.Helper()
	for _, product := range products {
		if product.Name == name {
			return product
		}
	}
	t.Fatalf("product %q not found in %+v", name, products)
	return Product{}
}

func findLicense(t *testing.T, licenses []License, product string) License {
	t.Helper()
	for _, license := range licenses {
		if license.Product == product {
			return license
		}
	}
	t.Fatalf("license for %q not found in %+v", product, licenses)
	return License{}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
