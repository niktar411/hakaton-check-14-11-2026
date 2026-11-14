package sam

import "time"

// Overview is the aggregate view of the software asset estate. Every value is
// computed by the database; the client never sends or derives these numbers.
type Overview struct {
	Products                int64 `json:"products" doc:"Software products in the catalog"`
	Assets                  int64 `json:"assets" doc:"Inventory assets"`
	Licenses                int64 `json:"licenses" doc:"License records"`
	SeatsTotal              int64 `json:"seatsTotal" doc:"Licensed seats across all licenses"`
	SeatsUsed               int64 `json:"seatsUsed" doc:"Assigned seats across all licenses"`
	OpenFindings            int64 `json:"openFindings" doc:"Findings that are not resolved"`
	PendingMatches          int64 `json:"pendingMatches" doc:"Suggested matches awaiting review"`
	ExpiringLicenses        int64 `json:"expiringLicenses" doc:"Licenses expiring within 30 days"`
	UnlicensedInstallations int64 `json:"unlicensedInstallations" doc:"Installations without a catalog product"`
}

// Product is one catalog entry with its current license and installation usage.
type Product struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Vendor            string    `json:"vendor"`
	Category          string    `json:"category"`
	LicenseCount      int64     `json:"licenseCount"`
	InstallationCount int64     `json:"installationCount"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// License is a contract for a catalog product. Status is derived by the server
// from the expiry date so every client sees the same state.
type License struct {
	ID         int64   `json:"id"`
	Product    string  `json:"product"`
	Vendor     string  `json:"vendor"`
	Kind       string  `json:"kind" enum:"subscription,perpetual,trial"`
	SeatsTotal int32   `json:"seatsTotal"`
	SeatsUsed  int32   `json:"seatsUsed"`
	ValidUntil *string `json:"validUntil" doc:"Calendar date in YYYY-MM-DD format"`
	Status     string  `json:"status" enum:"active,expiring,expired"`
}

// Asset is a managed device with the number of discovered installations.
type Asset struct {
	ID            int64      `json:"id"`
	Hostname      string     `json:"hostname"`
	Platform      string     `json:"platform"`
	Owner         string     `json:"owner"`
	SoftwareCount int64      `json:"softwareCount"`
	LastSeenAt    *time.Time `json:"lastSeenAt"`
}

// Finding is a compliance rule violation. Product and Asset are empty when the
// finding is not tied to a specific catalog entry or device.
type Finding struct {
	ID         int64     `json:"id"`
	Code       string    `json:"code"`
	Severity   string    `json:"severity" enum:"low,medium,high,critical"`
	Status     string    `json:"status" enum:"open,acknowledged,resolved"`
	Product    string    `json:"product"`
	Asset      string    `json:"asset"`
	Details    string    `json:"details"`
	DetectedAt time.Time `json:"detectedAt"`
}

// Match is a proposed link between a discovered installation and a catalog
// product. Confidence is a server value in the range 0..1.
type Match struct {
	ID           int64     `json:"id"`
	DetectedName string    `json:"detectedName"`
	Version      string    `json:"version"`
	Asset        string    `json:"asset"`
	Product      string    `json:"product"`
	Confidence   float64   `json:"confidence"`
	Status       string    `json:"status" enum:"suggested,confirmed,rejected"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Suggestion is a read-only recommendation derived from the current estate.
// The client translates Kind and links to Path; the server does not send prose.
type Suggestion struct {
	Kind  string `json:"kind" enum:"expiring_licenses,open_findings,pending_matches,unlicensed_installations"`
	Count int64  `json:"count"`
	Path  string `json:"path"`
}

// Assistant is the read-only assistant surface. It currently returns
// recommendations derived from real SAM aggregates.
type Assistant struct {
	Suggestions []Suggestion `json:"suggestions"`
}
