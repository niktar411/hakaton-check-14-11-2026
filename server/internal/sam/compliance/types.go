package compliance

import "time"

// Summary is the dashboard projection of the SAM compliance engine. All money
// values are whole rubles per year; the server derives them from license cost
// and never accepts prices from the client.
type Summary struct {
	Products               int64               `json:"products" doc:"Software products in the catalog"`
	ActiveProducts         int64               `json:"activeProducts" doc:"Products in active status"`
	BannedProducts         int64               `json:"bannedProducts" doc:"Products banned by policy"`
	Assets                 int64               `json:"assets" doc:"Managed hosts"`
	Licenses               int64               `json:"licenses" doc:"License records"`
	Installations          int64               `json:"installations" doc:"Discovered software installations"`
	Devices                int64               `json:"devices" doc:"Hosts with at least one installation"`
	UnmatchedInstallations int64               `json:"unmatchedInstallations" doc:"Installations without a catalog product"`
	PurchasedSeats         int64               `json:"purchasedSeats" doc:"Seats across all license records"`
	LicensedSeats          int64               `json:"licensedSeats" doc:"Seats on licenses valid today"`
	ExpiredSeats           int64               `json:"expiredSeats" doc:"Seats on licenses whose term has ended"`
	OverusedSeats          int64               `json:"overusedSeats" doc:"Installations beyond valid licensed seats"`
	IdleSeats              int64               `json:"idleSeats" doc:"Valid licensed seats without installations"`
	BannedInstallations    int64               `json:"bannedInstallations" doc:"Hosts running banned products"`
	AnnualCost             int64               `json:"annualCost" doc:"Annual cost of valid licensed seats, RUB"`
	OveruseCost            int64               `json:"overuseCost" doc:"Annual cost of overused seats, RUB"`
	IdleCost               int64               `json:"idleCost" doc:"Annual cost of idle seats, RUB"`
	ExpiredCost            int64               `json:"expiredCost" doc:"Annual cost of expired seats, RUB"`
	CostAtRisk             int64               `json:"costAtRisk" doc:"Overuse plus expired cost, RUB"`
	CompliancePercent      int64               `json:"compliancePercent" doc:"Share of products without licence violations, 0..100"`
	RiskLevel              string              `json:"riskLevel" enum:"low,medium,high"`
	Recommendations        []string            `json:"recommendations" doc:"Ordered actions derived from the current balance"`
	Categories             []CategoryBreakdown `json:"categories" doc:"Product distribution by category"`
	Statuses               []StatusBreakdown   `json:"statuses" doc:"Product distribution by catalog status"`
}

// CategoryBreakdown powers the category chart and its accessible table.
type CategoryBreakdown struct {
	Category         string `json:"category"`
	Products         int64  `json:"products"`
	InstalledDevices int64  `json:"installedDevices"`
	OverusedSeats    int64  `json:"overusedSeats"`
	RiskCost         int64  `json:"riskCost" doc:"Annual cost at risk in the category, RUB"`
}

// StatusBreakdown powers the catalog status chart.
type StatusBreakdown struct {
	Status           string `json:"status" enum:"active,inactive,archived,banned"`
	Products         int64  `json:"products"`
	InstalledDevices int64  `json:"installedDevices"`
}

// ComplianceProduct is one catalog entry with its purchased/installed/active balance.
type ComplianceProduct struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Vendor              string `json:"vendor"`
	Category            string `json:"category"`
	Status              string `json:"status" enum:"active,inactive,archived,banned"`
	Licenses            int64  `json:"licenses" doc:"License records for the product"`
	PurchasedSeats      int64  `json:"purchasedSeats" doc:"Seats across all license records"`
	LicensedSeats       int64  `json:"licensedSeats" doc:"Seats on licenses valid today"`
	ExpiredSeats        int64  `json:"expiredSeats" doc:"Seats on licenses whose term has ended"`
	InstalledDevices    int64  `json:"installedDevices" doc:"Unique hosts running the product"`
	Installations       int64  `json:"installations" doc:"Installation records"`
	BannedInstallations int64  `json:"bannedInstallations" doc:"Hosts running a banned product"`
	OverusedSeats       int64  `json:"overusedSeats" doc:"Installed devices beyond valid seats"`
	IdleSeats           int64  `json:"idleSeats" doc:"Valid seats without installations"`
	DeltaSeats          int64  `json:"deltaSeats" doc:"Installed devices minus valid licensed seats"`
	AnnualCost          int64  `json:"annualCost" doc:"Annual cost of valid seats, RUB"`
	AvgCostPerSeat      int64  `json:"avgCostPerSeat" doc:"Average annual seat price, RUB"`
	RiskCost            int64  `json:"riskCost" doc:"Annual cost of the balance violation, RUB"`
	RiskLevel           string `json:"riskLevel" enum:"low,medium,high"`
	Recommendation      string `json:"recommendation" doc:"Plain-language next action"`
}

// Risk is a flattened, severity-sorted compliance issue. ProductID is absent
// for estate-wide risks such as unmatched installations.
type Risk struct {
	Code           string `json:"code" enum:"banned_product,license_overuse,expired_license,idle_license,inactive_product,unmatched_installations"`
	Severity       string `json:"severity" enum:"medium,high"`
	ProductID      *int64 `json:"productId,omitempty"`
	ProductName    string `json:"productName,omitempty"`
	Category       string `json:"category,omitempty"`
	Affected       int64  `json:"affected" doc:"Seats, hosts or installations touched by the risk"`
	Cost           int64  `json:"cost" doc:"Annual cost of the risk, RUB"`
	Recommendation string `json:"recommendation"`
}

// ComplianceLicense is a license record shown on the product detail screen.
type ComplianceLicense struct {
	ID          int64   `json:"id"`
	Kind        string  `json:"kind" enum:"subscription,perpetual,trial"`
	SeatsTotal  int32   `json:"seatsTotal"`
	SeatsUsed   int32   `json:"seatsUsed"`
	CostPerSeat int64   `json:"costPerSeat" doc:"Annual seat price, RUB"`
	Currency    string  `json:"currency"`
	ValidUntil  *string `json:"validUntil" doc:"Calendar date in YYYY-MM-DD format"`
	Status      string  `json:"status" enum:"active,expiring,expired"`
}

// HostUsage links a product to one affected host.
type HostUsage struct {
	AssetID       int64      `json:"assetId"`
	Hostname      string     `json:"hostname"`
	Platform      string     `json:"platform"`
	Owner         string     `json:"owner"`
	Version       string     `json:"version"`
	LastSeenAt    *time.Time `json:"lastSeenAt"`
	LicenseStatus string     `json:"licenseStatus" enum:"licensed,expired,unlicensed,banned,inactive"`
}

// ComplianceProductDetail is the drill-down target for a compliance row.
type ComplianceProductDetail struct {
	Product  ComplianceProduct   `json:"product"`
	Licenses []ComplianceLicense `json:"licenses"`
	Hosts    []HostUsage         `json:"hosts"`
}

// HostInstallation is one discovered installation on a host detail screen.
type HostInstallation struct {
	ID           int64  `json:"id"`
	ProductID    *int64 `json:"productId,omitempty"`
	Product      string `json:"product"`
	Vendor       string `json:"vendor"`
	Category     string `json:"category"`
	DetectedName string `json:"detectedName"`
	Version      string `json:"version"`
	Status       string `json:"status" enum:"licensed,expired,unlicensed,banned,inactive"`
}

// HostDetail is the drill-down target for a managed host.
type HostDetail struct {
	ID            int64              `json:"id"`
	Hostname      string             `json:"hostname"`
	Platform      string             `json:"platform"`
	Owner         string             `json:"owner"`
	LastSeenAt    *time.Time         `json:"lastSeenAt"`
	Installations []HostInstallation `json:"installations"`
	Licensed      int64              `json:"licensed"`
	Expired       int64              `json:"expired"`
	Unlicensed    int64              `json:"unlicensed"`
	Banned        int64              `json:"banned"`
	Inactive      int64              `json:"inactive"`
}
