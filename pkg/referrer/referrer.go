package referrer

import (
	"net/url"
	"strings"
)

// Info contains parsed referrer attributes.
type Info struct {
	Name string `json:"name"` // e.g. "Google Search", "Instagram", "TikTok"
	Type string `json:"type"` // e.g. "search", "social", "internal", "direct"
	URL  string `json:"url"`
}

// TODO(accuracy): Port the complete search/social/ad directory from OpenPanel:
// `/Users/rakib/Projects/analytics/openpanel/packages/common/server/referrers/index.ts`
// (which contains over 2,800 host mappings for Google, Bing, DuckDuckGo, Baidu, Yandex,
// Facebook, Instagram, TikTok, Pinterest, Reddit, Twitter/X, LinkedIn, etc.).
// In Go, this can be compiled into an efficient static Trie or hash map (`map[string]ReferrerRule`)
// with sub-microsecond zero-alloc lookup.

// Parse resolves a referrer URL into a structured Name and Type.
func Parse(refURL string) Info {
	trimmed := strings.TrimSpace(refURL)
	if trimmed == "" {
		return Info{
			Type: "direct",
		}
	}

	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return Info{
			Name: trimmed,
			Type: "referral",
			URL:  trimmed,
		}
	}

	host := strings.ToLower(u.Host)
	cleanHost := strings.TrimPrefix(host, "www.")

	// Common search engines & social networks quick match
	// TODO: Replace with complete trie dictionary from openpanel/packages/common/server/referrers
	switch {
	case strings.Contains(cleanHost, "google."):
		return Info{Name: "Google", Type: "search", URL: trimmed}
	case strings.Contains(cleanHost, "bing.com"):
		return Info{Name: "Bing", Type: "search", URL: trimmed}
	case strings.Contains(cleanHost, "duckduckgo.com"):
		return Info{Name: "DuckDuckGo", Type: "search", URL: trimmed}
	case strings.Contains(cleanHost, "yahoo.com"):
		return Info{Name: "Yahoo", Type: "search", URL: trimmed}
	case strings.Contains(cleanHost, "facebook.com") || strings.Contains(cleanHost, "fb.com"):
		return Info{Name: "Facebook", Type: "social", URL: trimmed}
	case strings.Contains(cleanHost, "instagram.com"):
		return Info{Name: "Instagram", Type: "social", URL: trimmed}
	case strings.Contains(cleanHost, "tiktok.com"):
		return Info{Name: "TikTok", Type: "social", URL: trimmed}
	case strings.Contains(cleanHost, "twitter.com") || cleanHost == "t.co" || cleanHost == "x.com":
		return Info{Name: "X (Twitter)", Type: "social", URL: trimmed}
	case strings.Contains(cleanHost, "pinterest.com"):
		return Info{Name: "Pinterest", Type: "social", URL: trimmed}
	case strings.Contains(cleanHost, "linkedin.com"):
		return Info{Name: "LinkedIn", Type: "social", URL: trimmed}
	case strings.Contains(cleanHost, "youtube.com") || cleanHost == "youtu.be":
		return Info{Name: "YouTube", Type: "social", URL: trimmed}
	default:
		return Info{
			Name: cleanHost,
			Type: "referral",
			URL:  trimmed,
		}
	}
}

// ParseUTM extracts referrer name and type from query parameters (utm_source, ref, etc.).
// TODO(accuracy): Align with OpenPanel's `getReferrerWithQuery` in `openpanel/packages/common/server/parse-referrer.ts`.
func ParseUTM(query url.Values) *Info {
	source := strings.ToLower(strings.TrimSpace(query.Get("utm_source")))
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(query.Get("ref")))
	}
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(query.Get("utm_referrer")))
	}

	if source == "" {
		return nil
	}

	medium := strings.ToLower(strings.TrimSpace(query.Get("utm_medium")))
	refType := "referral"
	if strings.Contains(medium, "cpc") || strings.Contains(medium, "paid") || strings.Contains(medium, "ad") {
		refType = "paid"
	} else if strings.Contains(medium, "social") {
		refType = "social"
	} else if strings.Contains(medium, "email") {
		refType = "email"
	}

	return &Info{
		Name: source,
		Type: refType,
	}
}
