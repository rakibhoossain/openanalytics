package hash

import (
	"crypto/sha256"
	"encoding/hex"
)

// GenerateDeviceID produces a stable anonymous SHA-256 fingerprint from IP, User-Agent, and store salt.
func GenerateDeviceID(salt, shopID, ip, userAgent string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(":"))
	h.Write([]byte(shopID))
	h.Write([]byte(":"))
	h.Write([]byte(ip))
	h.Write([]byte(":"))
	h.Write([]byte(userAgent))
	return hex.EncodeToString(h.Sum(nil))
}
