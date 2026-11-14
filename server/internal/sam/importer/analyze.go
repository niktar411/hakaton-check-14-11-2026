package importer

import "strings"

// Analyze filters non-trackable updates, rejects rows without a name or with
// missing publisher/version, and collapses duplicates by device+name+version.
// The accepted slice preserves the input order, which keeps an upsert stable.
func Analyze(rows []Row) ([]Row, Report) {
	report := Report{Total: len(rows)}
	accepted := make([]Row, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))

	for _, row := range rows {
		if IsUpdate(row) {
			report.Updates++
			continue
		}
		if strings.TrimSpace(row.Name) == "" {
			report.Errors++
			report.Issues = append(report.Issues, Issue{
				Line:    row.Line,
				Field:   "name",
				Code:    "row.empty_name",
				Message: "software name is required",
			})
			continue
		}

		missingPublisher := strings.TrimSpace(row.Publisher) == ""
		missingVersion := strings.TrimSpace(row.Version) == ""
		if missingPublisher {
			report.EmptyPublisher++
		}
		if missingVersion {
			report.EmptyVersion++
		}
		if missingPublisher || missingVersion {
			report.Empty++
			report.Issues = append(report.Issues, Issue{
				Line:    row.Line,
				Field:   missingField(missingPublisher, missingVersion),
				Code:    "row.empty_publisher_or_version",
				Message: "publisher and version are required for catalog matching",
			})
			continue
		}

		key := DedupeKey(row)
		if _, exists := seen[key]; exists {
			report.Duplicates++
			continue
		}
		seen[key] = struct{}{}
		accepted = append(accepted, row)
	}

	report.Accepted = len(accepted)
	return accepted, report
}

func missingField(missingPublisher, missingVersion bool) string {
	switch {
	case missingPublisher && missingVersion:
		return "publisher,version"
	case missingPublisher:
		return "publisher"
	default:
		return "version"
	}
}
