// Package importer normalizes software inventory exports without coupling to
// the database schema. The SAM tables are owned by a separate migration, so
// callers can validate and deduplicate a CSV before any catalog exists.
package importer

// Row is one inventory record after column mapping. Line is the 1-based source
// line, preserved for the import report.
type Row struct {
	Line         int
	Device       string
	Name         string
	Publisher    string
	Version      string
	Architecture string
	Edition      string
	Update       string
	InstallDate  string
	Source       string
}

// Issue is a non-fatal problem found while parsing or validating a row.
type Issue struct {
	Line    int    `json:"line,omitempty"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Report summarizes what happened to every input row. The invariant is:
// Total == Accepted + Duplicates + Empty + Updates + Errors.
type Report struct {
	Total          int     `json:"total"`
	Accepted       int     `json:"accepted"`
	Duplicates     int     `json:"duplicates"`
	Empty          int     `json:"empty"`
	EmptyPublisher int     `json:"emptyPublisher"`
	EmptyVersion   int     `json:"emptyVersion"`
	Updates        int     `json:"updates"`
	Errors         int     `json:"errors"`
	Issues         []Issue `json:"issues,omitempty"`
}

// Import is the result of a full pass: the rows ready for an idempotent upsert
// plus the report shown to the operator.
type Import struct {
	Accepted []Row
	Report   Report
}

// ParseResult separates successfully decoded rows from recoverable CSV issues.
// An error is returned only when the header or delimiter cannot be understood.
type ParseResult struct {
	Rows   []Row
	Issues []Issue
}
