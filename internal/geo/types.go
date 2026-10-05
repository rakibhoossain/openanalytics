package geo

// Location contains geographical metadata resolved from an IP address.
type Location struct {
	Country     string   `json:"country,omitempty"`      // ISO 2-letter country code, e.g. "US"
	CountryCode string   `json:"country_code,omitempty"` // ISO 2-letter country code alias
	CountryName string   `json:"country_name,omitempty"` // Full country name, e.g. "United States"
	City        string   `json:"city,omitempty"`         // City name, e.g. "New York"
	Region      string   `json:"region,omitempty"`       // State or region code, e.g. "NY"
	RegionCode  string   `json:"region_code,omitempty"`  // State or region code alias
	RegionName  string   `json:"region_name,omitempty"`  // State or region full name, e.g. "New York"
	PostalCode  string   `json:"postal_code,omitempty"`  // Postal/ZIP code, e.g. "10001"
	Continent   string   `json:"continent,omitempty"`    // Continent code, e.g. "NA", "EU", "AS"
	Longitude   *float32 `json:"longitude,omitempty"`
	Latitude    *float32 `json:"latitude,omitempty"`
	Timezone    string   `json:"timezone,omitempty"`
}

// ASNInfo contains Autonomous System Number metadata for bot/datacenter classification.
type ASNInfo struct {
	AutonomousSystemNumber       uint   `json:"asn,omitempty"`
	AutonomousSystemOrganization string `json:"aso,omitempty"`
	IsDatacenter                 bool   `json:"is_datacenter"`
}
