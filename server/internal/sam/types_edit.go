package sam

import "time"

// Формы редактирования SAM. Все значения проверяет сервер: счётчики
// (лицензии, установки, места) — производные, из запроса они не приходят.

// ProductInput is the editable product form. Status uses the internal
// category (banned/outdated), registry fields follow the Russian software
// registry.
type ProductInput struct {
	Name          string `json:"name" minLength:"1" maxLength:"200"`
	Vendor        string `json:"vendor" maxLength:"200" required:"false"`
	Version       string `json:"version" maxLength:"100" required:"false"`
	Category      string `json:"category" maxLength:"300" required:"false"`
	Status        string `json:"status" enum:"active,outdated,banned,archived" required:"false"`
	SectionCode   string `json:"sectionCode" maxLength:"10" required:"false"`
	ClassCode     string `json:"classCode" maxLength:"10" required:"false"`
	LicenseMetric string `json:"licenseMetric" enum:"device,user,installation,none" required:"false"`
	RegistryRf    bool   `json:"registryRf" required:"false"`
	RegistryEntry string `json:"registryEntry" maxLength:"50" required:"false"`
	IsTrackable   bool   `json:"isTrackable" required:"false"`
}

// LicenseInput is the editable license form. seatsUsed is derived from
// installations; costPerSeat and currency carry the money side.
type LicenseInput struct {
	ProductID   int64   `json:"productId"`
	Kind        string  `json:"kind" enum:"subscription,perpetual,open_source,trial" required:"false"`
	SeatsTotal  int32   `json:"seatsTotal" minimum:"0" maximum:"1000000" required:"false"`
	Contract    string  `json:"contract" maxLength:"120" required:"false"`
	CostPerSeat int64   `json:"costPerSeat" minimum:"0" required:"false"`
	Currency    string  `json:"currency" maxLength:"8" required:"false"`
	ValidUntil  *string `json:"validUntil" doc:"Calendar date YYYY-MM-DD or null"`
}

// InstallationInput is the manually recorded installation. ProductID links
// the record to a catalog entry; the matching engine may re-link it later.
type InstallationInput struct {
	RawName    string `json:"rawName" minLength:"1" maxLength:"200"`
	DeviceName string `json:"deviceName" maxLength:"200" required:"false"`
	UserName   string `json:"userName" maxLength:"200" required:"false"`
	Version    string `json:"version" maxLength:"100" required:"false"`
	ProductID  *int64 `json:"productId"`
	Status     string `json:"status" enum:"installed,removed,unknown" required:"false"`
}

// AliasInput adds a known alias for a product; the matching engine uses
// normalized aliases to clean dirty process names.
type AliasInput struct {
	Alias string `json:"alias" minLength:"1" maxLength:"200"`
}

// Alias is a known product alias, public read shape.
type Alias struct {
	ID    int64  `json:"id"`
	Alias string `json:"alias"`
}

// Installation is one recorded installation row, public read shape.
type Installation struct {
	ID         int64     `json:"id"`
	ProductID  *int64    `json:"productId"`
	Product    string    `json:"product"`
	RawName    string    `json:"rawName"`
	DeviceName string    `json:"deviceName"`
	UserName   string    `json:"userName"`
	Version    string    `json:"version"`
	Source     string    `json:"source" enum:"manual,inventory"`
	Status     string    `json:"status" enum:"installed,removed,unknown" required:"false"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}
