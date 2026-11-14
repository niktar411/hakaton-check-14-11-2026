package importer

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseCSVFixture(t *testing.T) {
	file, err := os.Open("testdata/inventory.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer file.Close()

	parsed, err := ParseCSV(file)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(parsed.Issues) != 0 {
		t.Fatalf("unexpected issues: %+v", parsed.Issues)
	}
	if len(parsed.Rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(parsed.Rows))
	}
	first := parsed.Rows[0]
	if first.Device != "PC-001" || first.Name != "Microsoft Visual C++ 2015-2022 Redistributable (x64)" {
		t.Fatalf("unexpected first row: %+v", first)
	}
	if first.Publisher != "Microsoft Corporation" || first.Version != "14.38.33130" {
		t.Fatalf("unexpected first row publisher/version: %+v", first)
	}
	if parsed.Rows[4].Update != "KB5034441" {
		t.Fatalf("update column not mapped: %+v", parsed.Rows[4])
	}
}

func TestAnalyzeFixtureCounts(t *testing.T) {
	file, err := os.Open("testdata/inventory.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer file.Close()

	result, err := ImportCSV(file)
	if err != nil {
		t.Fatalf("import fixture: %v", err)
	}
	if result.Report.Total != 6 || result.Report.Updates != 1 || result.Report.Empty != 1 || result.Report.Duplicates != 1 {
		t.Fatalf("unexpected report: %+v", result.Report)
	}
	if result.Report.Accepted != 3 || len(result.Accepted) != 3 {
		t.Fatalf("accepted = %d, want 3", result.Report.Accepted)
	}
	for _, row := range result.Accepted {
		if row.Name == "Office 2019" {
			t.Fatalf("row with empty version was accepted: %+v", row)
		}
	}
}

func TestParseCSVDirectiveBOMAndEnglishHeaders(t *testing.T) {
	input := "\ufeffsep=;\r\nDevice Name;Software Name;Vendor;Product Version;Architecture\r\nPC-9;App;Vendor;1.0;x64\r\n"
	parsed, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(parsed.Rows))
	}
	row := parsed.Rows[0]
	if row.Device != "PC-9" || row.Name != "App" || row.Publisher != "Vendor" || row.Version != "1.0" || row.Architecture != "x64" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.Line != 3 {
		t.Fatalf("line = %d, want 3 (separator directive + header)", row.Line)
	}
}

func TestParseCSVReportsBadRows(t *testing.T) {
	input := "Device,Name,Publisher,Version\nPC-1,App,Vendor,1.0\nPC-2,Too,Few\n,,\n"
	parsed, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(parsed.Rows))
	}
	codes := map[string]int{}
	for _, issue := range parsed.Issues {
		codes[issue.Code]++
	}
	if codes["row.columns"] != 1 || codes["row.empty"] != 1 {
		t.Fatalf("unexpected issues: %+v", parsed.Issues)
	}
}

func TestParseCSVMissingNameColumn(t *testing.T) {
	_, err := ParseCSV(strings.NewReader("Device,Publisher,Version\nPC-1,Vendor,1.0\n"))
	if err == nil {
		t.Fatal("expected a missing name column error")
	}
}

func TestNormalizeName(t *testing.T) {
	tests := map[string]string{
		"Microsoft Visual C++ 2019 (x64)": "microsoft visual c plus plus 2019",
		"  MICROSOFT® Word™ ":             "microsoft word",
		"7-Zip (x86_64)":                  "7 zip",
		"C#":                              "c sharp",
		"Node.js":                         "node js",
		"Компас-3D":                       "kompas 3d",
	}
	for input, want := range tests {
		if got := NormalizeName(input); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeVendor(t *testing.T) {
	tests := map[string]string{
		"Microsoft Corporation":      "microsoft",
		"Google LLC":                 "google",
		"The Qt Company":             "qt",
		"Adobe Systems Incorporated": "adobe systems",
		"ООО «Лаборатория Касперского»": "laboratoriya kasperskogo",
	}
	for input, want := range tests {
		if got := NormalizeVendor(input); got != want {
			t.Errorf("NormalizeVendor(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := map[string]string{
		"v1.02.0.0":          "1.2",
		"v.1.0":              "1",
		"1.2.3.0":            "1.2.3",
		"Version 10.0.19045": "10.0.19045",
		"1.2.3 (x64)":        "1.2.3",
		"2024":               "2024",
		"":                   "",
	}
	for input, want := range tests {
		if got := NormalizeVersion(input); got != want {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeArchitecture(t *testing.T) {
	tests := map[string]string{
		"x86_64": "amd64", "x64": "amd64", "Intel 64-bit": "amd64",
		"i686": "386", "32-bit": "386",
		"aarch64": "arm64", "ARM64": "arm64",
		"all": "any", "NoArch": "any",
	}
	for input, want := range tests {
		if got := NormalizeArchitecture(input); got != want {
			t.Errorf("NormalizeArchitecture(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsUpdate(t *testing.T) {
	tests := []struct {
		name string
		row  Row
		want bool
	}{
		{"kb in name", Row{Name: "Security Update for Windows (KB5034441)"}, true},
		{"kb in update column", Row{Name: "Windows 11", Update: "KB5034441"}, true},
		{"spaced kb in update column", Row{Name: "Windows 11", Update: "KB 5034441"}, true},
		{"cumulative update", Row{Name: "2023-11 Cumulative Update for Windows 11"}, true},
		{"update for", Row{Name: "Microsoft .NET Framework 4.8.1 Update for Windows"}, true},
		{"hotfix", Row{Name: "Hotfix package"}, true},
		{"service pack", Row{Name: "Windows Server 2019 Service Pack 1"}, true},
		{"update manager is a product", Row{Name: "VMware Update Manager"}, false},
		{"java update numbering is a product", Row{Name: "Java 8 Update 411"}, false},
		{"no update marker", Row{Name: "Windows 10", Update: "No"}, false},
		{"normal product", Row{Name: "7-Zip"}, false},
	}
	for _, test := range tests {
		if got := IsUpdate(test.row); got != test.want {
			t.Errorf("%s: IsUpdate(%+v) = %v, want %v", test.name, test.row, got, test.want)
		}
	}
}

func TestDedupeKey(t *testing.T) {
	first := Row{Device: "PC-001", Name: "Microsoft Visual C++ (x64)", Version: "v1.0.0"}
	second := Row{Device: "pc001.corp.local", Name: "MICROSOFT VISUAL C++", Version: "1"}
	if DedupeKey(first) != DedupeKey(second) {
		t.Fatalf("keys differ: %q != %q", DedupeKey(first), DedupeKey(second))
	}
	otherVersion := second
	otherVersion.Version = "1.1"
	if DedupeKey(first) == DedupeKey(otherVersion) {
		t.Fatal("different versions produced the same key")
	}
	otherDevice := second
	otherDevice.Device = "PC-002"
	if DedupeKey(first) == DedupeKey(otherDevice) {
		t.Fatal("different devices produced the same key")
	}
}

func TestAnalyzeProductionScaleCounts(t *testing.T) {
	rows := make([]Row, 0, 9084)
	for i := 0; i < 8650; i++ {
		rows = append(rows, Row{
			Line:   len(rows) + 1,
			Device: fmt.Sprintf("UPDATE-%d", i),
			Name:   "Security Update for Windows (KB" + strconv.Itoa(5000000+i) + ")",
		})
	}
	for i := 0; i < 420; i++ {
		row := Row{
			Line:      len(rows) + 1,
			Device:    fmt.Sprintf("EMPTY-%d", i),
			Name:      fmt.Sprintf("Application %d", i),
			Publisher: "Vendor",
			Version:   "1.0",
		}
		if i%2 == 0 {
			row.Publisher = ""
		} else {
			row.Version = ""
		}
		rows = append(rows, row)
	}
	for i := 0; i < 10; i++ {
		rows = append(rows, Row{
			Line:      len(rows) + 1,
			Device:    fmt.Sprintf("PC-%02d", i),
			Name:      fmt.Sprintf("Product %d", i),
			Publisher: "Vendor",
			Version:   "1.0",
		})
	}
	duplicate := Row{Device: "PC-DUP", Name: "Duplicate Product", Publisher: "Vendor", Version: "2.0"}
	rows = append(rows, duplicate, duplicate, duplicate)
	rows = append(rows, Row{Line: len(rows) + 1, Device: "PC-EMPTY-NAME", Publisher: "Vendor", Version: "1.0"})

	accepted, report := Analyze(rows)
	if report.Updates != 8650 {
		t.Fatalf("updates = %d, want 8650", report.Updates)
	}
	if report.Empty != 420 || report.EmptyPublisher != 210 || report.EmptyVersion != 210 {
		t.Fatalf("empty counts = %+v, want 420/210/210", report)
	}
	if report.Duplicates != 2 {
		t.Fatalf("duplicates = %d, want 2", report.Duplicates)
	}
	if report.Errors != 1 {
		t.Fatalf("errors = %d, want 1", report.Errors)
	}
	if report.Accepted != 11 || len(accepted) != 11 {
		t.Fatalf("accepted = %d, want 11", report.Accepted)
	}
	if report.Total != report.Accepted+report.Duplicates+report.Empty+report.Updates+report.Errors {
		t.Fatalf("report invariant broken: %+v", report)
	}
}

func TestImportCSV24500RowsUnder30Seconds(t *testing.T) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"Device", "Name", "Publisher", "Version", "Architecture"}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	for i := 0; i < 24500; i++ {
		err := writer.Write([]string{
			fmt.Sprintf("PC-%05d", i),
			fmt.Sprintf("Product %d", i),
			"Vendor",
			fmt.Sprintf("1.%d.0", i),
			"x64",
		})
		if err != nil {
			t.Fatalf("write row %d: %v", i, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatalf("flush CSV: %v", err)
	}

	started := time.Now()
	result, err := ImportCSV(&buffer)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Report.Total != 24500 || result.Report.Accepted != 24500 {
		t.Fatalf("unexpected report: %+v", result.Report)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("import took %s, want under 30s", elapsed)
	}
}
