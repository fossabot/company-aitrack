package service

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// tokenPrefix is the mandatory prefix on every raw aitrack token.
const tokenPrefix = "aitrack_"

// ComputeTokenKey strips the "aitrack_" prefix then returns first-6 + "…" + last-4.
// The token_key is the non-secret display identifier for a credential.
func ComputeTokenKey(rawToken string) string {
	stripped := rawToken
	if strings.HasPrefix(rawToken, tokenPrefix) {
		stripped = rawToken[len(tokenPrefix):]
	}
	if len(stripped) <= 10 {
		return stripped
	}
	return stripped[:6] + "…" + stripped[len(stripped)-4:]
}

// NewRawToken generates a fresh raw token of the form "aitrack_<64 hex chars>".
func NewRawToken() string {
	return tokenPrefix + RandomHex(32)
}

// RandomHex returns 2n lowercase hex characters of cryptographic randomness.
func RandomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
