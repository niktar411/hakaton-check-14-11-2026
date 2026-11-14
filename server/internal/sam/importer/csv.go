package importer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

var (
	// ErrHeader means the file has no readable header row.
	ErrHeader = errors.New("sam importer: missing CSV header")
	// ErrMissingNameColumn means the header has no recognizable product name.
	ErrMissingNameColumn = errors.New("sam importer: product name column not found")
)

// ParseCSV reads an inventory export, maps common English and Russian column
// names to Row fields, and reports recoverable row problems separately. Only a
// broken header or an unreadable stream stops the whole import.
func ParseCSV(r io.Reader) (ParseResult, error) {
	reader := bufio.NewReaderSize(r, 64*1024)
	if prefix, err := reader.Peek(3); err == nil && bytes.Equal(prefix, []byte{0xEF, 0xBB, 0xBF}) {
		if _, err := reader.Discard(3); err != nil {
			return ParseResult{}, fmt.Errorf("skip UTF-8 BOM: %w", err)
		}
	}

	first, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return ParseResult{}, fmt.Errorf("read header: %w", err)
	}
	if strings.TrimSpace(first) == "" {
		return ParseResult{}, ErrHeader
	}

	headerLine := first
	lineOffset := 0
	delimiter, hasDirective := separatorDirective(first)
	if hasDirective {
		lineOffset = 1
		headerLine, err = reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return ParseResult{}, fmt.Errorf("read header after separator: %w", err)
		}
		if strings.TrimSpace(headerLine) == "" {
			return ParseResult{}, ErrHeader
		}
	} else {
		delimiter = detectDelimiter(headerLine)
	}

	parsed := csv.NewReader(io.MultiReader(strings.NewReader(headerLine), reader))
	parsed.Comma = delimiter
	parsed.FieldsPerRecord = -1
	parsed.LazyQuotes = true
	parsed.TrimLeadingSpace = true

	header, err := parsed.Read()
	if err != nil {
		return ParseResult{}, fmt.Errorf("parse header: %w", err)
	}
	if len(header) == 0 {
		return ParseResult{}, ErrHeader
	}

	columns := mapColumns(header)
	if _, ok := columns["name"]; !ok {
		return ParseResult{}, ErrMissingNameColumn
	}

	result := ParseResult{}
	for {
		record, readErr := parsed.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			var parseErr *csv.ParseError
			if errors.As(readErr, &parseErr) {
				result.Issues = append(result.Issues, Issue{
					Line:    parseErr.Line + lineOffset,
					Code:    "csv.parse",
					Message: parseErr.Err.Error(),
				})
				continue
			}
			result.Issues = append(result.Issues, Issue{Code: "csv.read", Message: readErr.Error()})
			break
		}
		if isEmptyRecord(record) {
			result.Issues = append(result.Issues, Issue{Code: "row.empty", Message: "empty row"})
			continue
		}
		line, _ := parsed.FieldPos(0)
		line += lineOffset
		if len(record) != len(header) {
			result.Issues = append(result.Issues, Issue{
				Line:    line,
				Code:    "row.columns",
				Message: fmt.Sprintf("expected %d columns, got %d", len(header), len(record)),
			})
			continue
		}

		row := Row{
			Line:         line,
			Device:       columnValue(record, columns, "device"),
			Name:         columnValue(record, columns, "name"),
			Publisher:    columnValue(record, columns, "publisher"),
			Version:      columnValue(record, columns, "version"),
			Architecture: columnValue(record, columns, "architecture"),
			Edition:      columnValue(record, columns, "edition"),
			Update:       columnValue(record, columns, "update"),
			InstallDate:  columnValue(record, columns, "install_date"),
			Source:       columnValue(record, columns, "source"),
		}
		if !validUTF8Row(row) {
			result.Issues = append(result.Issues, Issue{
				Line:    line,
				Code:    "row.encoding",
				Message: "row is not valid UTF-8",
			})
			continue
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

// ImportCSV parses, filters and deduplicates an inventory in one call.
func ImportCSV(r io.Reader) (Import, error) {
	parsed, err := ParseCSV(r)
	if err != nil {
		return Import{}, err
	}
	accepted, report := Analyze(parsed.Rows)
	if len(parsed.Issues) > 0 {
		report.Total += len(parsed.Issues)
		report.Errors += len(parsed.Issues)
		report.Issues = append(parsed.Issues, report.Issues...)
	}
	return Import{Accepted: accepted, Report: report}, nil
}

// headerAliases maps a normalized header to the canonical Row field. Keys are
// lowercase, punctuation-free and transliterated, so Russian headers work too.
var headerAliases = map[string]string{
	"device": "device", "devicename": "device", "computer": "device", "computername": "device",
	"computernamefqdn": "device", "host": "device", "hostname": "device", "machine": "device",
	"machinename": "device", "workstation": "device", "workstationname": "device", "asset": "device",
	"assetname": "device", "node": "device", "nodename": "device", "ustroystvo": "device",
	"imyaustroistva": "device", "ustroistvo": "device", "ustroistva": "device",
	"kompyuter": "device", "imyakompyutera": "device",

	"name": "name", "software": "name", "softwarename": "name", "product": "name",
	"productname": "name", "displayname": "name", "application": "name", "applicationname": "name",
	"program": "name", "programname": "name", "appname": "name", "title": "name",
	"nazvanie": "name", "po": "name", "programma": "name", "naimenovanie": "name",
	"naimenovaniepo": "name",

	"publisher": "publisher", "vendor": "publisher", "manufacturer": "publisher",
	"softwarepublisher": "publisher", "company": "publisher", "izdatel": "publisher",
	"proizvoditel": "publisher",

	"version": "version", "productversion": "version", "displayversion": "version",
	"softwareversion": "version", "ver": "version", "versiya": "version",

	"architecture": "architecture", "arch": "architecture", "osarchitecture": "architecture",
	"platform": "architecture", "bitness": "architecture", "arhitektura": "architecture",
	"razryadnost": "architecture",

	"edition": "edition", "release": "edition", "sku": "edition", "redakciya": "edition",

	"update": "update", "updateid": "update", "kb": "update", "kbid": "update",
	"hotfix": "update", "patch": "update", "patchid": "update", "obnovlenie": "update",

	"installdate": "install_date", "installedon": "install_date", "installed": "install_date",
	"installationdate": "install_date", "dataustanovki": "install_date", "datainstall": "install_date",

	"source": "source", "sourcefile": "source", "inventorysource": "source",
	"scanner": "source", "istochnik": "source",
}

func mapColumns(header []string) map[string]int {
	columns := make(map[string]int, len(headerAliases))
	for index, value := range header {
		key := canonicalHeader(value)
		if key == "" {
			continue
		}
		canonical, ok := headerAliases[key]
		if !ok {
			continue
		}
		if _, exists := columns[canonical]; !exists {
			columns[canonical] = index
		}
	}
	return columns
}

func canonicalHeader(value string) string {
	return strings.ReplaceAll(NormalizeName(value), " ", "")
}

func columnValue(record []string, columns map[string]int, name string) string {
	index, ok := columns[name]
	if !ok || index < 0 || index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

func validUTF8Row(row Row) bool {
	return utf8.ValidString(row.Device) && utf8.ValidString(row.Name) &&
		utf8.ValidString(row.Publisher) && utf8.ValidString(row.Version) &&
		utf8.ValidString(row.Architecture) && utf8.ValidString(row.Edition) &&
		utf8.ValidString(row.Update) && utf8.ValidString(row.InstallDate) &&
		utf8.ValidString(row.Source)
}

func isEmptyRecord(record []string) bool {
	if len(record) == 0 {
		return true
	}
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func separatorDirective(line string) (rune, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 5 || !strings.EqualFold(trimmed[:4], "sep=") {
		return 0, false
	}
	delimiter, _ := utf8.DecodeRuneInString(trimmed[4:])
	if strings.ContainsRune(",;\t|", delimiter) {
		return delimiter, true
	}
	return 0, false
}

func detectDelimiter(header string) rune {
	candidates := []rune{',', ';', '\t', '|'}
	best := ','
	bestCount := 0
	for _, candidate := range candidates {
		count := countOutsideQuotes(header, candidate)
		if count > bestCount {
			best = candidate
			bestCount = count
		}
	}
	return best
}

func countOutsideQuotes(line string, target rune) int {
	inQuotes := false
	count := 0
	for _, r := range line {
		if r == '"' {
			inQuotes = !inQuotes
			continue
		}
		if r == target && !inQuotes {
			count++
		}
	}
	return count
}
