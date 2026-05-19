// Package service holds pure domain business logic: signature computation,
// diff consistency, field validation, edit-profile analytics and prompt
// classification. Nothing here depends on HTTP, SQL, or configuration files.
package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// SignatureService provides HMAC-SHA256 and SHA-256 helpers.
// All hex output is lowercase — matches Rust client and Java server conventions.
type SignatureService struct{}

// NewSignatureService constructs a SignatureService.
func NewSignatureService() *SignatureService { return &SignatureService{} }

// SHA256Hex returns the lowercase hex SHA-256 digest of data.
func (s *SignatureService) SHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// SHA256HexStr returns the lowercase hex SHA-256 digest of a string.
func (s *SignatureService) SHA256HexStr(data string) string {
	return s.SHA256Hex([]byte(data))
}

// HmacSHA256Hex returns the lowercase hex HMAC-SHA256 of message under secret.
func (s *SignatureService) HmacSHA256Hex(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// ComputeRequestSignature computes X-AiTrack-Signature.
// canonical = timestamp + "\n" + sha256_hex(rawBodyBytes)
func (s *SignatureService) ComputeRequestSignature(hmacSecret, timestamp string, rawBody []byte) string {
	bodyHash := s.SHA256Hex(rawBody)
	canonical := timestamp + "\n" + bodyHash
	return s.HmacSHA256Hex(hmacSecret, canonical)
}

// ComputeRecordSig computes the per-record record_sig.
// Field order MUST match CONTRACT.md and Rust client exactly.
// v1.1: token_key, device_id, hostname, timestamp, tool, file_path, repo_url,
// current_sha, added_lines, removed_lines, sha256(diff_hunk)
func (s *SignatureService) ComputeRecordSig(
	hmacSecret, tokenKey, deviceID, hostname, timestamp, tool, filePath, repoURL, currentSHA string,
	addedLines, removedLines int64,
	diffHunk *string,
) string {
	hunkStr := ""
	if diffHunk != nil {
		hunkStr = *diffHunk
	}
	diffHunkHash := s.SHA256HexStr(hunkStr)

	canonical := tokenKey + "\n" +
		deviceID + "\n" +
		hostname + "\n" +
		timestamp + "\n" +
		tool + "\n" +
		filePath + "\n" +
		repoURL + "\n" +
		currentSHA + "\n" +
		fmt.Sprintf("%d", addedLines) + "\n" +
		fmt.Sprintf("%d", removedLines) + "\n" +
		diffHunkHash

	return s.HmacSHA256Hex(hmacSecret, canonical)
}

// ConstantTimeEqual compares two strings without timing leaks, including length.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
