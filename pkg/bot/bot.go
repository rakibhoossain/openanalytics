package bot

import (
	"net/http"
	"strings"

	"openanalytics/internal/geo"
	"openanalytics/pkg/uaparser"
)

// Suspicion represents the outcome of bot heuristic analysis.
type Suspicion struct {
	IsBot   bool     `json:"is_bot"`
	Reasons []string `json:"reasons,omitempty"`
}

// TODO(accuracy): Expand bot detection to achieve full parity with OpenPanel:
// `/Users/rakib/Projects/analytics/openpanel/apps/api/src/bots/suspicion.ts` and
// `/Users/rakib/Projects/analytics/openpanel/apps/api/src/bots/header-signals.ts`:
//   1. Multi-category scoring: Only flag when >= 2 distinct categories fire (e.g. Datacenter IP + Missing Sec-CH-UA).
//   2. Client Hints verification: Inspect `Sec-CH-UA`, `Sec-CH-UA-Mobile`, `Sec-CH-UA-Platform`.
//   3. Fetch Metadata checks: `Sec-Fetch-Site`, `Sec-Fetch-Mode`, `Sec-Fetch-Dest`.
//   4. Missing standard browser headers (e.g. missing `Accept-Language` or `Accept-Encoding`).
//   5. Whitelist trusted server-to-server webhook traffic (verified via store API keys).

// Detect evaluates an HTTP request, ASN metadata, and User-Agent to determine bot suspicion.
func Detect(r *http.Request, asnInfo *geo.ASNInfo, uaInfo uaparser.UAInfo) Suspicion {
	var reasons []string

	// 1. User-Agent heuristic
	if uaInfo.IsBot {
		reasons = append(reasons, "ua:bot_pattern")
	}

	// 2. Datacenter IP heuristic
	if asnInfo != nil && asnInfo.IsDatacenter {
		reasons = append(reasons, "datacenter_ip")
	}

	// 3. Header anomaly heuristics (TODO: port complete header-signals.ts)
	if r.Header.Get("User-Agent") == "" {
		reasons = append(reasons, "header:missing_user_agent")
	}
	if r.Header.Get("Accept-Language") == "" && !uaInfo.IsBot {
		reasons = append(reasons, "header:missing_accept_language")
	}

	// Flag as bot if User-Agent is explicitly a bot or if multiple anomalies occur
	isBot := uaInfo.IsBot || len(reasons) >= 2

	return Suspicion{
		IsBot:   isBot,
		Reasons: reasons,
	}
}

// ApplyToProperties annotates an event's properties with server-side authoritative bot metadata.
func ApplyToProperties(props map[string]string, s Suspicion) map[string]string {
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
