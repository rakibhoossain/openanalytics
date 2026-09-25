package query

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rakibhoossain/ua-parser-go/referrer"
)

type memoryFaviconCache struct {
	sync.RWMutex
	items map[string]cachedFavicon
}

type cachedFavicon struct {
	data        []byte
	contentType string
	expiresAt   time.Time
}

var (
	memFaviconCache = &memoryFaviconCache{
		items: make(map[string]cachedFavicon),
	}
	httpClient = &http.Client{
		Timeout: 6 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			return nil
		},
	}
)

func isPrivateHost(host string) bool {
	h := strings.Split(host, ":")[0]
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func isDirectImageURL(u *url.URL) bool {
	path := strings.ToLower(u.Path)
	for _, ext := range []string{".svg", ".png", ".jpg", ".jpeg", ".ico", ".webp", ".gif"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	raw := strings.ToLower(u.String())
	return strings.Contains(raw, "googleusercontent.com") || strings.Contains(raw, "wikimedia.org")
}

func detectImageType(data []byte, path string) string {
	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, ".svg") || strings.Contains(string(data[:min(len(data), 100)]), "<svg"):
		return "image/svg+xml"
	case strings.HasSuffix(lowerPath, ".ico") || (len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0):
		return "image/x-icon"
	case strings.HasSuffix(lowerPath, ".png") || (len(data) >= 8 && string(data[1:4]) == "PNG"):
		return "image/png"
	case strings.HasSuffix(lowerPath, ".webp") || (len(data) >= 12 && string(data[8:12]) == "WEBP"):
		return "image/webp"
	case strings.HasSuffix(lowerPath, ".jpg") || strings.HasSuffix(lowerPath, ".jpeg") || (len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8):
		return "image/jpeg"
	default:
		cType := http.DetectContentType(data)
		if strings.HasPrefix(cType, "image/") {
			return cType
		}
		return "image/x-icon"
	}
}

func downloadURL(targetURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	// Use realistic browser user-agent to bypass strict CDN blocks like Wikimedia
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty body received")
	}

	return body, nil
}

func fetchFaviconWithFallbacks(u *url.URL) ([]byte, string) {
	hostname := u.Hostname()

	// 1. If it's a direct image URL (e.g. Wikimedia SVGs/PNGs for browsers and OS)
	if isDirectImageURL(u) {
		if data, err := downloadURL(u.String()); err == nil && len(data) > 0 {
			return data, detectImageType(data, u.Path)
		}
	}

	// 2. Fallback 1: DuckDuckGo favicon service
	if hostname != "" {
		ddgURL := fmt.Sprintf("https://icons.duckduckgo.com/ip3/%s.ico", hostname)
		if data, err := downloadURL(ddgURL); err == nil && len(data) > 0 {
			// DuckDuckGo returns an empty or tiny placeholder if not found, usually ~1459 or less with empty glyphs
			// but if valid data returned, accept
			return data, detectImageType(data, ".ico")
		}
	}

	// 3. Fallback 2: Google Favicon service via ua-parser-go referrer helper
	if hostname != "" {
		gURL := referrer.GoogleFaviconURL(hostname, 64)
		if gURL != "" {
			if data, err := downloadURL(gURL); err == nil && len(data) > 0 {
				return data, detectImageType(data, ".png")
			}
		}
	}

	// 4. Fallback 3: Standard /favicon.ico at origin
	originFavicon := fmt.Sprintf("%s://%s/favicon.ico", u.Scheme, u.Host)
	if data, err := downloadURL(originFavicon); err == nil && len(data) > 0 {
		return data, "image/x-icon"
	}

	return nil, ""
}

func serveImage(w http.ResponseWriter, data []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// HandleFaviconProxy proxies and caches favicons, browser icons, and referrer logos.
func (h *Handler) HandleFaviconProxy(w http.ResponseWriter, r *http.Request) {
	rawTarget := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawTarget == "" {
		http.Error(w, "url query parameter is required", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(rawTarget, "http://") && !strings.HasPrefix(rawTarget, "https://") {
		rawTarget = "https://" + rawTarget
	}

	targetURL, err := url.Parse(rawTarget)
	if err != nil || targetURL.Host == "" {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	if isPrivateHost(targetURL.Host) {
		http.Error(w, "forbidden destination host", http.StatusForbidden)
		return
	}

	hash := sha256.Sum256([]byte(rawTarget))
	cacheKey := "favicon:v2:" + hex.EncodeToString(hash[:])
	typeKey := cacheKey + ":ctype"

	// 1. Check L1 In-Memory Cache
	memFaviconCache.RLock()
	if item, found := memFaviconCache.items[cacheKey]; found && time.Now().Before(item.expiresAt) {
		memFaviconCache.RUnlock()
		serveImage(w, item.data, item.contentType)
		return
	}
	memFaviconCache.RUnlock()

	// 2. Check Redis Cache
	ctx := r.Context()
	if h.rdb != nil {
		if cachedData, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cachedData) > 0 {
			cType, _ := h.rdb.Get(ctx, typeKey).Result()
			if cType == "" {
				cType = detectImageType(cachedData, targetURL.Path)
			}
			// Warm L1 memory cache
			memFaviconCache.Lock()
			memFaviconCache.items[cacheKey] = cachedFavicon{
				data:        cachedData,
				contentType: cType,
				expiresAt:   time.Now().Add(24 * time.Hour),
			}
			memFaviconCache.Unlock()

			serveImage(w, cachedData, cType)
			return
		}
	}

	// 3. Fetch Image with fallbacks
	imgData, cType := fetchFaviconWithFallbacks(targetURL)
	if len(imgData) == 0 {
		http.Error(w, "favicon not found", http.StatusNotFound)
		return
	}

	// 4. Save to Redis Cache (7 days TTL)
	if h.rdb != nil {
		_ = h.rdb.Set(ctx, cacheKey, imgData, 7*24*time.Hour).Err()
		_ = h.rdb.Set(ctx, typeKey, cType, 7*24*time.Hour).Err()
	}

	// Save to L1 Memory Cache
	memFaviconCache.Lock()
	memFaviconCache.items[cacheKey] = cachedFavicon{
		data:        imgData,
		contentType: cType,
		expiresAt:   time.Now().Add(24 * time.Hour),
	}
	memFaviconCache.Unlock()

	serveImage(w, imgData, cType)
}

// HandleFaviconClear flushes the favicon proxy cache.
func (h *Handler) HandleFaviconClear(w http.ResponseWriter, r *http.Request) {
	memFaviconCache.Lock()
	memFaviconCache.items = make(map[string]cachedFavicon)
	memFaviconCache.Unlock()

	if h.rdb != nil {
		ctx := r.Context()
		keys, _ := h.rdb.Keys(ctx, "favicon:v2:*").Result()
		if len(keys) > 0 {
			_ = h.rdb.Del(ctx, keys...).Err()
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"cleared"}`))
}
