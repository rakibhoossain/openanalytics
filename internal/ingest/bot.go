package ingest

import (
	"net/http"
	"strings"

	"openanalytics/internal/geo"
	uaparser "github.com/rakibhoossain/ua-parser-go"
)

// BotSuspicion represents the outcome of multi-factor bot heuristic analysis.
type BotSuspicion struct {
	IsBot   bool     `json:"is_bot"`
	Reasons []string `json:"reasons,omitempty"`
}

// detectBotSuspicion evaluates an HTTP request, ASN datacenter metadata, and User-Agent to determine bot suspicion.
func detectBotSuspicion(r *http.Request, asnInfo *geo.ASNInfo, uaRes *uaparser.Result) BotSuspicion {
	var reasons []string

	// 1. Authoritative User-Agent bot pattern check
	if uaRes.IsBot {
		reasons = append(reasons, "ua:bot_pattern")
	}

	// 2. Datacenter IP heuristic (AWS, GCP, Cloudflare, etc.)
	if asnInfo != nil && asnInfo.IsDatacenter {
		reasons = append(reasons, "datacenter_ip")
	}

	// 3. Header anomaly heuristics
	if r.Header.Get("User-Agent") == "" {
		reasons = append(reasons, "header:missing_user_agent")
	}
	if r.Header.Get("Accept-Language") == "" && !uaRes.IsBot {
		reasons = append(reasons, "header:missing_accept_language")
	}

	// Flag as bot if User-Agent is explicitly a bot or if multiple anomalies occur
	isBot := uaRes.IsBot || len(reasons) >= 2

	return BotSuspicion{
		IsBot:   isBot,
		Reasons: reasons,
	}
}

// applyBotVerdict annotates an event's properties with server-side authoritative bot metadata.
func applyBotVerdict(props map[string]string, s BotSuspicion) map[string]string {
	if props == nil {
		props = make(map[string]string)
	}

	// Strip any client-supplied spoofed bot flags
	delete(props, "__bot")
	delete(props, "__bot_reasons")

	if s.IsBot {
		props["__bot"] = "1"
	}
	if len(s.Reasons) > 0 {
		props["__bot_reasons"] = strings.Join(s.Reasons, ",")
	}

	return props
}
