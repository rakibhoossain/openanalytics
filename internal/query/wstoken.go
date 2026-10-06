package query

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var defaultWSSecret = "aicart_openanalytics_ws_jwt_secret"

func getWSSecret() []byte {
	if s := os.Getenv("WS_JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte(defaultWSSecret)
}

// WSTokenClaims holds the authenticated tenant and shop payload for WebSocket connections.
type WSTokenClaims struct {
	ShopID   string `json:"shop_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Exp      int64  `json:"exp"`
}

// GenerateWSToken creates an HMAC-SHA256 JWT string valid for the given duration.
func GenerateWSToken(shopID string, tenantID string, ttl time.Duration) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := WSTokenClaims{
		ShopID:   shopID,
		TenantID: tenantID,
		Exp:      time.Now().Add(ttl).Unix(),
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := header + "." + payload

	mac := hmac.New(sha256.New, getWSSecret())
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

// ValidateWSToken verifies the signature, expiration, and matches the target shopID.
func ValidateWSToken(tokenString string, expectedShopID string) (*WSTokenClaims, error) {
	if tokenString == "" {
		return nil, errors.New("missing websocket token")
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, getWSSecret())
	mac.Write([]byte(signingInput))
	expectedSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSignature)) {
		return nil, errors.New("invalid token signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid token payload")
	}

	var claims WSTokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("failed to parse token claims")
	}

	if time.Now().Unix() > claims.Exp {
		return nil, errors.New("websocket token expired")
	}

	if expectedShopID != "" && claims.ShopID != "" && !strings.EqualFold(claims.ShopID, expectedShopID) {
		return nil, fmt.Errorf("token shop_id %s does not match requested shop %s", claims.ShopID, expectedShopID)
	}

	return &claims, nil
}
