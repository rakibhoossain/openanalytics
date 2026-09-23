package uaparser

import (
	"strings"

	"github.com/mileusna/useragent"
)

// UAInfo contains parsed User-Agent properties.
type UAInfo struct {
	OS             string `json:"os"`
	OSVersion      string `json:"os_version"`
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	Device         string `json:"device"`
	IsBot          bool   `json:"is_bot"`
}

// TODO(accuracy): Build a dedicated Go port/binding for `/Users/rakib/Projects/analytics/ua-parser-js`
// or compile its regex test suites to Go to achieve 100% regex parity with OpenPanel's client SDKs.
// Also incorporate custom server-side overrides from `openpanel/packages/common/server/parser-user-agent.ts`,
// including:
//   - Custom browser/device overrides (e.g. Electron apps, headless Chrome, specific WebView wrappers)
//   - Device brand/model extraction (e.g. Apple iPhone 15, Samsung Galaxy S23)
//   - Client Hints processing (sec-ch-ua, sec-ch-ua-mobile, sec-ch-ua-platform) for high-accuracy Chrome/Edge on mobile
// Parse extracts structured platform, OS, browser, and device properties from a User-Agent header.
func Parse(uaStr string) UAInfo {
	if uaStr == "" {
		return UAInfo{
			Device: "unknown",
		}
	}

	ua := useragent.Parse(uaStr)

	device := "desktop"
	if ua.Mobile {
		device = "mobile"
	} else if ua.Tablet {
		device = "tablet"
	} else if ua.Bot {
		device = "bot"
	}

	// Extra bot heuristic check
	isBot := ua.Bot || isBotString(uaStr)
	if isBot {
		device = "bot"
	}

	return UAInfo{
		OS:             ua.OS,
		OSVersion:      ua.OSVersion,
		Browser:        ua.Name,
		BrowserVersion: ua.Version,
		Device:         device,
		IsBot:          isBot,
	}
}

func isBotString(ua string) bool {
	lower := strings.ToLower(ua)
	botSubstrings := []string{
		"bot", "crawler", "spider", "slurp", "facebookexternalhit",
		"googlebot", "bingbot", "yandex", "duckduckbot", "headless",
	}
	for _, sub := range botSubstrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}
