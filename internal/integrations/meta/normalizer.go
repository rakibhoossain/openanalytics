package meta

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// HashSHA256 returns the lowercase hexadecimal SHA-256 hash of the input string.
func HashSHA256(input string) string {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(hash[:])
}

// NormalizeEmail trims whitespace, converts to lowercase, and hashes via SHA-256.
func NormalizeEmail(email string) string {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	if trimmed == "" {
		return ""
	}
	return HashSHA256(trimmed)
}

// NormalizePhone removes all non-digit characters, removes country-specific leading zeros,
// prepends country code if missing or formats as E.164, and hashes via SHA-256.
func NormalizePhone(phone string) string {
	cleaned := strings.TrimSpace(phone)
	if cleaned == "" {
		return ""
	}

	var digits strings.Builder
	for _, r := range cleaned {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	rawDigits := digits.String()
	if rawDigits == "" {
		return ""
	}

	// Remove leading zeros (e.g. 01712... -> 1712...)
	rawDigits = strings.TrimLeft(rawDigits, "0")
	if rawDigits == "" {
		return ""
	}

	return HashSHA256(rawDigits)
}

// NormalizeAlpha trims, converts to lowercase, strips punctuation, and hashes.
// Used for first name, last name, and city.
func NormalizeAlpha(s string) string {
	cleaned := strings.ToLower(strings.TrimSpace(s))
	if cleaned == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range cleaned {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	res := b.String()
	if res == "" {
		return ""
	}
	return HashSHA256(res)
}

// NormalizeCountry formats as 2-letter lowercase ISO 3166-1 alpha-2 code and hashes.
func NormalizeCountry(country string) string {
	c := strings.ToLower(strings.TrimSpace(country))
	if len(c) > 2 {
		c = c[:2]
	}
	if c == "" {
		return ""
	}
	return HashSHA256(c)
}

// NormalizeZip formats zip/postal code (lowercase, stripped whitespace/dashes) and hashes.
func NormalizeZip(zip string) string {
	z := strings.ToLower(strings.TrimSpace(zip))
	z = strings.ReplaceAll(z, " ", "")
	z = strings.ReplaceAll(z, "-", "")
	if z == "" {
		return ""
	}
	return HashSHA256(z)
}
