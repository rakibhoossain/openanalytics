package ingest

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	uaparser "github.com/rakibhoossain/ua-parser-go"
)

// LookupRequest represents the input parameters for readonly IP and User-Agent resolution.
type LookupRequest struct {
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

// LookupBrowser contains parsed browser metadata.
type LookupBrowser struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// LookupOS contains parsed operating system metadata.
type LookupOS struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// LookupDevice contains hardware device metadata.
type LookupDevice struct {
	Type   string `json:"type"`
	Vendor string `json:"vendor,omitempty"`
	Model  string `json:"model,omitempty"`
}

// LookupLocation contains minimal geographical coordinates and address.
type LookupLocation struct {
	City      string   `json:"city,omitempty"`
	Country   string   `json:"country,omitempty"`
	Longitude *float32 `json:"longitude,omitempty"`
	Latitude  *float32 `json:"latitude,omitempty"`
}

// LookupResponse is the minimal, clean response for login session metadata.
type LookupResponse struct {
	IP       string          `json:"ip"`
	ASO      string          `json:"aso,omitempty"`
	Browser  LookupBrowser   `json:"browser"`
	OS       LookupOS        `json:"os"`
	Device   LookupDevice    `json:"device"`
	Location *LookupLocation `json:"location,omitempty"`
}

// HandleLookup is a strictly read-only endpoint that resolves address and device info from {ip, user_agent}.
// Performs zero database writes, zero Redis writes, and zero Kafka publishes.
func (h *Handler) HandleLookup(w http.ResponseWriter, r *http.Request) {
	var req LookupRequest

	// Read optional JSON body on POST
	if r.Method == http.MethodPost && r.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if err == nil && len(bodyBytes) > 0 && strings.TrimSpace(string(bodyBytes)) != "" {
			_ = json.Unmarshal(bodyBytes, &req)
		}
	}

	// 1. Resolve IP (payload > header)
	ipStr := strings.TrimSpace(req.IP)
	if ipStr == "" {
		ipStr = h.extractClientIP(r)
	}

	// 2. Resolve User-Agent (payload > header)
	uaStr := strings.TrimSpace(req.UserAgent)
	if uaStr == "" {
		uaStr = strings.TrimSpace(r.Header.Get("User-Agent"))
	}

	// 3. Inspect IP properties
	parsedIP := net.ParseIP(ipStr)
	isPrivate := false
	if parsedIP != nil {
		isPrivate = parsedIP.IsLoopback() || parsedIP.IsPrivate() || parsedIP.IsLinkLocalUnicast()
	} else if isLocalIP(ipStr) {
		isPrivate = true
	}

	// 4. Resolve Geographical Address & ASN (read-only from in-memory MMDB)
	var lookupLoc *LookupLocation
	var aso string
	if h.geoService != nil && !isPrivate && ipStr != "" {
		if loc, _ := h.geoService.Lookup(ipStr); loc != nil {
			country := loc.CountryName
			if country == "" {
				country = loc.Country
			}
			lookupLoc = &LookupLocation{
				City:      loc.City,
				Country:   country,
				Longitude: loc.Longitude,
				Latitude:  loc.Latitude,
			}
		}
		if asnInfo, _ := h.geoService.LookupASN(ipStr); asnInfo != nil {
			aso = asnInfo.AutonomousSystemOrganization
		}
	}

	// 5. Parse User-Agent (read-only tokenized parser)
	var uaRes *uaparser.Result
	if uaStr != "" {
		uaRes = uaparser.Parse(uaStr)
	} else {
		uaRes = uaparser.ParseRequest(r)
		uaStr = uaRes.UA
	}

	devType := resolveDeviceType(uaRes)

	resp := LookupResponse{
		IP:  ipStr,
		ASO: aso,
		Browser: LookupBrowser{
			Name:    uaRes.Browser.Name,
			Version: uaRes.Browser.Version,
		},
		OS: LookupOS{
			Name:    uaRes.OS.Name,
			Version: uaRes.OS.Version,
		},
		Device: LookupDevice{
			Type:   devType,
			Vendor: uaRes.Device.Vendor,
			Model:  uaRes.Device.Model,
		},
		Location: lookupLoc,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
