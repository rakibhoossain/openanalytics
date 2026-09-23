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

// KnownCrawlers contains lowercase patterns for major search engine spiders and scrapers.
// Matches OpenPanel's isBotHook to prevent web crawlers from polluting conversion funnels.
var KnownCrawlers = []string{
	"googlebot", "bingbot", "bingpreview", "slurp", "duckduckbot", "baiduspider",
	"yandexbot", "sogou", "exabot", "facebot", "facebookexternalhit", "twitterbot",
	"linkedinbot", "pinterest", "applebot", "semrushbot", "ahrefsbot", "mj12bot",
	"dotbot", "petalbot", "screaming frog", "seznambot", "archive.org_bot", "ia_archiver",
	"bytespider", "gptbot", "chatgpt-user", "ccbot", "anthropic-ai", "claude-web",
}

// IsKnownCrawler returns true if the User-Agent represents a known search spider or web crawler.
func IsKnownCrawler(userAgent string) bool {
	if userAgent == "" {
		return false
	}
	lower := strings.ToLower(userAgent)
	for _, pattern := range KnownCrawlers {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

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
