package ingest

import (
	"fmt"
	"net/http"
	"strings"

	"openanalytics/internal/geo"
	uaparser "github.com/rakibhoossain/ua-parser-go"
	"github.com/rakibhoossain/ua-parser-go/bots"
)

// BotSuspicion represents the outcome of multi-factor bot heuristic analysis.
type BotSuspicion = bots.BotSuspicion

// detectBotSuspicion evaluates an HTTP request, ASN datacenter metadata, and User-Agent to determine bot suspicion.
func detectBotSuspicion(r *http.Request, asnInfo *geo.ASNInfo, uaRes *uaparser.Result) BotSuspicion {
	var reasons []string

	// 1. Authoritative User-Agent bot pattern check
	if uaRes != nil && uaRes.IsBot {
		reasons = append(reasons, "ua:bot_pattern")
	}

	// 2. Datacenter IP heuristic
	if asnInfo != nil && asnInfo.IsDatacenter {
		asnStr := fmt.Sprintf("%d", asnInfo.AutonomousSystemNumber)
		if asnStr == "0" {
			asnStr = "unknown"
		}
		reasons = append(reasons, "datacenter_ip:AS"+asnStr)
	}

	// 3. Request Header anomalies
	if r != nil {
		ua := ""
		if uaRes != nil {
			ua = uaRes.UA
		} else {
			ua = r.Header.Get("User-Agent")
		}
		reasons = append(reasons, bots.DetectHeaderAnomalies(r.Header, ua)...)
	}

	verdict := bots.SummarizeSignals(reasons)
	if uaRes != nil && uaRes.IsBot {
		verdict.IsBot = true
	}
	return verdict
}

// applyBotVerdict annotates an event's properties with server-side authoritative bot metadata.
func applyBotVerdict(props map[string]string, s BotSuspicion) map[string]string {
	if props == nil {
		props = make(map[string]string)
	}

	bots.StripBotProperties(props)

	if s.IsBot {
		props["__bot"] = "1"
	}
	if len(s.Reasons) > 0 {
		props["__bot_reasons"] = strings.Join(s.Reasons, ",")
	}

	return props
}
