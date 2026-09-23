package geo

// Location contains geographical metadata resolved from an IP address.
type Location struct {
	Country   string   `json:"country,omitempty"`   // ISO 2-letter country code, e.g. "US"
	City      string   `json:"city,omitempty"`      // City name, e.g. "New York"
	Region    string   `json:"region,omitempty"`    // State or region code/name, e.g. "NY"
	Longitude *float32 `json:"longitude,omitempty"`
	Latitude  *float32 `json:"latitude,omitempty"`
	Timezone  string   `json:"timezone,omitempty"`
}

// ASNInfo contains Autonomous System Number metadata for bot/datacenter classification.
type ASNInfo struct {
	AutonomousSystemNumber       uint   `json:"asn,omitempty"`
	AutonomousSystemOrganization string `json:"aso,omitempty"`
	IsDatacenter                 bool   `json:"is_datacenter"`
}
