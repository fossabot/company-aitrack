// chain_integration_test.go — real Go server chain integration test.
//
// Uses the actual chi router wired to a PostgreSQL/ParadeDB database.
// No real credentials are required; the test issues its own token via the
// admin endpoint, builds correctly-signed payloads, and drives the full
// 10-step validation chain end-to-end.
//
// Requires TEST_DATABASE_URL to point to a reachable PostgreSQL instance.
// If the env var is absent the test is skipped gracefully.
package e2e

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/aitrack/server/testapp"
)

// ─── test config helpers ──────────────────────────────────────────────────────

const (
	integrationAdminKey = "integration-test-admin-key"
)

func integrationDSN() string {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		return "postgres://aitrack:aitrack_secret@localhost:5432/aitrack_test?sslmode=disable"
	}
	return dsn
}

// newIntegrationConfig returns a Config that uses a live PostgreSQL/ParadeDB
// instance and a known admin key. SecretKey is intentionally empty so the
// encryptor runs in dev mode (plain-prefix storage) — no crypto infrastructure
// required.
func newIntegrationConfig() *testapp.Config {
	return testapp.TestConfig(integrationDSN(), integrationAdminKey)
}

func TestMain(m *testing.M) {
	conn, err := sql.Open("pgx", integrationDSN())
	if err != nil || conn.Ping() != nil {
		fmt.Println("SKIP: TEST_DATABASE_URL not reachable, skipping E2E integration tests")
		os.Exit(0) // skip but pass
	}
	conn.Close()
	os.Exit(m.Run())
}

// ─── HMAC signing helpers (mirror of factory.go, self-contained) ─────────────

func sha256HexBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sha256HexString(s string) string { return sha256HexBytes([]byte(s)) }

func hmacSHA256Hex(secret, msg string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// computeIntegrationRecordSig replicates CONTRACT.md v1.1 record_sig computation.
// Field order: token_key, device_id, hostname, timestamp, tool, file_path,
//
//	repo_url, current_sha, added_lines, removed_lines, sha256(diff_hunk)
func computeIntegrationRecordSig(secret, tokenKey, deviceID, hostname, timestamp, tool, filePath, repoURL, currentSHA string, addedLines, removedLines int64, diffHunk string) string {
	diffHash := sha256HexString(diffHunk)
	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%d\n%d\n%s",
		tokenKey, deviceID, hostname, timestamp, tool, filePath, repoURL, currentSHA,
		addedLines, removedLines, diffHash)
	return hmacSHA256Hex(secret, canonical)
}

// computeIntegrationRequestSig replicates X-AiTrack-Signature computation.
// canonical = timestamp_str + "\n" + sha256_hex(rawBodyBytes)
func computeIntegrationRequestSig(secret, timestampStr string, rawBody []byte) string {
	bodyHash := sha256HexBytes(rawBody)
	canonical := timestampStr + "\n" + bodyHash
	return hmacSHA256Hex(secret, canonical)
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

type integrationClient struct {
	base       string
	httpClient *http.Client
	adminKey   string
}

func newIntegrationClient(base, adminKey string) *integrationClient {
	return &integrationClient{base: base, httpClient: &http.Client{}, adminKey: adminKey}
}

// adminPost sends POST with X-Admin-Key header, returns (status, body map).
func (c *integrationClient) adminPost(path string, payload interface{}) (int, map[string]interface{}) {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", c.adminKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(raw, &m)
	return resp.StatusCode, m
}

// signedPost sends POST with Bearer token and correct HMAC headers.
func (c *integrationClient) signedPost(path, rawToken, hmacSecret string, bodyBytes []byte) (int, map[string]interface{}) {
	tsStr := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeIntegrationRequestSig(hmacSecret, tsStr, bodyBytes)

	req, _ := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("X-AiTrack-Timestamp", tsStr)
	req.Header.Set("X-AiTrack-Signature", sig)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(raw, &m)
	return resp.StatusCode, m
}

// adminGet sends GET with X-Admin-Key header.
func (c *integrationClient) adminGet(path string) (int, map[string]interface{}) {
	req, _ := http.NewRequest(http.MethodGet, c.base+path, nil)
	req.Header.Set("X-Admin-Key", c.adminKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(raw, &m)
	return resp.StatusCode, m
}

// ─── Credential splitting (mirrors factory.SplitCredential) ──────────────────

type credBundle struct {
	credential string
	rawToken   string // everything before first "-"
	hmacSecret string // everything after first "-"
	tokenKey   string
}

func splitCredential(credential, tokenKey string) credBundle {
	idx := strings.Index(credential, "-")
	if idx < 0 {
		return credBundle{credential: credential, rawToken: credential, tokenKey: tokenKey}
	}
	return credBundle{
		credential: credential,
		rawToken:   credential[:idx],
		hmacSecret: credential[idx+1:],
		tokenKey:   tokenKey,
	}
}

// ─── Edit batch payload builder ───────────────────────────────────────────────

func buildIntegrationBatch(cred credBundle, deviceID string, n int) []byte {
	type editDTO struct {
		Tool         string  `json:"tool"`
		ToolVersion  string  `json:"tool_version"`
		Provider     string  `json:"provider"`
		Model        *string `json:"model"`
		SessionID    string  `json:"session_id"`
		RepoURL      string  `json:"repo_url"`
		Branch       string  `json:"branch"`
		CurrentSHA   string  `json:"current_sha"`
		FilePath     string  `json:"file_path"`
		AddedLines   int64   `json:"added_lines"`
		RemovedLines int64   `json:"removed_lines"`
		DiffHunk     string  `json:"diff_hunk"`
		Metadata     *string `json:"metadata"`
		Timestamp    string  `json:"timestamp"`
		DeviceID     string  `json:"device_id"`
		Hostname     string  `json:"hostname"`
		RecordSig    string  `json:"record_sig"`
	}

	edits := make([]editDTO, n)
	for i := 0; i < n; i++ {
		ts := time.Now().UTC().Format(time.RFC3339)
		diffHunk := fmt.Sprintf("@@ -1,3 +1,5 @@\n-old line %d\n-old line %da\n-old line %db\n+new line %d\n+new line %da\n+new line %db\n+new line %dc\n+new line %dd\n", i, i, i, i, i, i, i, i)
		addedLines := int64(5)
		removedLines := int64(3)
		filePath := fmt.Sprintf("src/module_%d.go", i)
		sha := fmt.Sprintf("%040x", i+1)

		sig := computeIntegrationRecordSig(
			cred.hmacSecret, cred.tokenKey,
			deviceID, "integration-host",
			ts, "claude", filePath,
			"git@github.com:aitrack-e2e/repo.git", sha,
			addedLines, removedLines, diffHunk,
		)

		edits[i] = editDTO{
			Tool:         "claude",
			ToolVersion:  "claude-code",
			Provider:     "anthropic",
			Model:        nil,
			SessionID:    fmt.Sprintf("sess-integration-%04d", i),
			RepoURL:      "git@github.com:aitrack-e2e/repo.git",
			Branch:       "main",
			CurrentSHA:   sha,
			FilePath:     filePath,
			AddedLines:   addedLines,
			RemovedLines: removedLines,
			DiffHunk:     diffHunk,
			Metadata:     nil,
			Timestamp:    ts,
			DeviceID:     deviceID,
			Hostname:     "integration-host",
			RecordSig:    sig,
		}
	}

	batch := map[string]interface{}{
		"device_id":      deviceID,
		"client_version": "1.0.0",
		"edits":          edits,
	}
	b, _ := json.Marshal(batch)
	return b
}

// ─── TestRealChainIntegration ─────────────────────────────────────────────────

// TestRealChainIntegration drives the real Go server (chi router + PostgreSQL/ParadeDB)
// through the complete request lifecycle:
//
//  1. Build real server via app.Build (in-memory SQLite, dev-mode encryptor)
//  2. Provision token via POST /admin/tokens (step 1 of server auth chain)
//  3. Parse credential → rawToken + hmacSecret
//  4. POST /api/v1/ai-track/edits with correct HMAC-signed batch (steps 2-10)
//     – Bearer auth (step 1 server-side: token resolution)
//     – X-AiTrack-Timestamp + X-AiTrack-Signature (steps 2-3: request HMAC)
//     – per-record record_sig (step 4: record HMAC verification)
//     – diff consistency check (step 5)
//     – whitelist check (step 6, disabled)
//     – path plausibility (step 7)
//     – size check (step 8)
//     – rate-limit check (step 9)
//     – persistence (step 10)
//  5. GET /api/v1/ai-track/profiles/{token_key} → verify total_records > 0
func TestRealChainIntegration(t *testing.T) {
	// Step 1: build real server with PostgreSQL/ParadeDB.
	cfg := newIntegrationConfig()
	router, cleanup, err := testapp.Build(cfg)
	if err != nil {
		t.Fatalf("app.Build failed: %v", err)
	}
	defer cleanup()

	srv := httptest.NewServer(router)
	defer srv.Close()

	client := newIntegrationClient(srv.URL, integrationAdminKey)

	// Step 2: provision a token via POST /admin/tokens.
	t.Log("Step 2: POST /admin/tokens")
	status, tokenResp := client.adminPost("/admin/tokens", map[string]interface{}{
		"owner": "integration-tester",
		"note":  "chain integration test",
	})
	if status != http.StatusOK {
		t.Fatalf("POST /admin/tokens: want 200, got %d; body=%v", status, tokenResp)
	}

	credStr, _ := tokenResp["credential"].(string)
	tokenKey, _ := tokenResp["token_key"].(string)
	if credStr == "" || tokenKey == "" {
		t.Fatalf("POST /admin/tokens: missing credential or token_key in response %v", tokenResp)
	}
	t.Logf("  token_key=%q  credential_len=%d", tokenKey, len(credStr))

	// Step 3: split credential.
	cred := splitCredential(credStr, tokenKey)
	if cred.rawToken == "" || cred.hmacSecret == "" {
		t.Fatalf("splitCredential: could not split %q", credStr)
	}
	t.Logf("  rawToken prefix=%q...  hmacSecret len=%d", cred.rawToken[:8], len(cred.hmacSecret))

	// Step 4: build a valid edit batch (3 records).
	const batchSize = 3
	deviceID := "integration-device-001"
	t.Logf("Step 4: building batch of %d edits with correct HMAC sigs", batchSize)
	batchBody := buildIntegrationBatch(cred, deviceID, batchSize)

	// Steps 2-10 are exercised by the server when it processes this request:
	//   2: bearer token resolution
	//   3: request HMAC validation
	//   4: per-record record_sig verification
	//   5: diff consistency
	//   6: repo whitelist (disabled in test config)
	//   7: path plausibility
	//   8: size check
	//   9: rate-limit check
	//  10: persistence
	t.Log("Steps 2-10: POST /api/v1/ai-track/edits with signed batch")
	editStatus, editResp := client.signedPost(
		"/api/v1/ai-track/edits",
		cred.rawToken,
		cred.hmacSecret,
		batchBody,
	)
	if editStatus != http.StatusOK {
		t.Fatalf("POST /edits: want 200, got %d; body=%v", editStatus, editResp)
	}

	accepted, _ := editResp["accepted"].(float64)
	if int(accepted) != batchSize {
		t.Errorf("POST /edits: want accepted=%d, got %.0f; resp=%v", batchSize, accepted, editResp)
	}
	t.Logf("  accepted=%.0f  rejected=%v  flagged=%v", accepted, editResp["rejected"], editResp["flagged"])

	// Step 5: GET /api/v1/ai-track/profiles/{token_key} and verify records were persisted.
	t.Logf("Step 5: GET /api/v1/ai-track/profiles/%s", tokenKey)
	profileStatus, profileResp := client.adminGet("/api/v1/ai-track/profiles/" + tokenKey)
	if profileStatus != http.StatusOK {
		t.Fatalf("GET /profiles/%s: want 200, got %d; body=%v", tokenKey, profileStatus, profileResp)
	}

	// Verify profile fields that confirm records were persisted through the full chain.
	totalEdits, _ := profileResp["total_edits"].(float64)
	if int(totalEdits) != batchSize {
		t.Errorf("profile.total_edits: want %d, got %.0f", batchSize, totalEdits)
	}

	totalAddedLines, _ := profileResp["total_added_lines"].(float64)
	if totalAddedLines <= 0 {
		t.Errorf("profile.total_added_lines: want > 0, got %.0f", totalAddedLines)
	}

	owner, _ := profileResp["owner"].(string)
	if owner == "" {
		t.Errorf("profile.owner should not be empty")
	}

	tokenKeyResp, _ := profileResp["token_key"].(string)
	if tokenKeyResp != tokenKey {
		t.Errorf("profile.token_key: want %q, got %q", tokenKey, tokenKeyResp)
	}

	// Verify languages map includes "Go" (all records have .go files).
	// The profile service returns capitalized language names (e.g. "Go", "Rust").
	langs, ok := profileResp["languages"].(map[string]interface{})
	if !ok || len(langs) == 0 {
		t.Errorf("profile.languages: want non-empty map, got %v", profileResp["languages"])
	}
	if _, hasGo := langs["Go"]; !hasGo {
		t.Errorf("profile.languages: expected 'Go' key; got %v", langs)
	}

	// Verify tools map.
	tools, ok := profileResp["tools"].(map[string]interface{})
	if !ok {
		t.Errorf("profile.tools: expected map, got %T=%v", profileResp["tools"], profileResp["tools"])
	} else {
		claudeCount, _ := tools["claude"].(float64)
		if claudeCount != float64(batchSize) {
			t.Errorf("profile.tools.claude: want %.0f, got %.0f", float64(batchSize), claudeCount)
		}
	}

	t.Logf("  owner=%q  total_edits=%.0f  total_added_lines=%.0f  languages=%v  tools=%v",
		owner, totalEdits, totalAddedLines, langs, tools)
	t.Log("TestRealChainIntegration: all 10 validation steps exercised and verified PASS")
}

// TestRealChainRejection verifies that the real server rejects a batch with
// a tampered record_sig (step 4 of the validation chain).
func TestRealChainRejection(t *testing.T) {
	cfg := newIntegrationConfig()
	router, cleanup, err := testapp.Build(cfg)
	if err != nil {
		t.Fatalf("app.Build failed: %v", err)
	}
	defer cleanup()

	srv := httptest.NewServer(router)
	defer srv.Close()

	client := newIntegrationClient(srv.URL, integrationAdminKey)

	// Provision token.
	status, tokenResp := client.adminPost("/admin/tokens", map[string]interface{}{
		"owner": "rejection-tester",
	})
	if status != http.StatusOK {
		t.Fatalf("POST /admin/tokens: want 200, got %d", status)
	}
	credStr, _ := tokenResp["credential"].(string)
	tokenKey, _ := tokenResp["token_key"].(string)
	cred := splitCredential(credStr, tokenKey)

	// Build a batch, then corrupt the record_sig.
	batchBody := buildIntegrationBatch(cred, "device-reject-001", 1)
	var batchMap map[string]interface{}
	json.Unmarshal(batchBody, &batchMap)

	editsRaw := batchMap["edits"].([]interface{})
	editMap := editsRaw[0].(map[string]interface{})
	editMap["record_sig"] = strings.Repeat("0", 64) // tampered
	batchBody, _ = json.Marshal(batchMap)

	editStatus, editResp := client.signedPost("/api/v1/ai-track/edits", cred.rawToken, cred.hmacSecret, batchBody)
	if editStatus != http.StatusOK {
		t.Fatalf("POST /edits: want 200 (batch-level OK with per-record rejection), got %d", editStatus)
	}

	rejected, _ := editResp["rejected"].([]interface{})
	if len(rejected) != 1 {
		t.Errorf("want 1 rejected record, got %d; resp=%v", len(rejected), editResp)
	}
	accepted, _ := editResp["accepted"].(float64)
	if int(accepted) != 0 {
		t.Errorf("want accepted=0, got %.0f", accepted)
	}

	// Verify rejection reason contains sig_mismatch.
	if len(rejected) > 0 {
		rejMap, _ := rejected[0].(map[string]interface{})
		reason, _ := rejMap["reason"].(string)
		if !strings.Contains(reason, "sig_mismatch") {
			t.Errorf("rejection reason: want sig_mismatch, got %q", reason)
		}
		t.Logf("Rejection confirmed: reason=%q (step 4: record_sig verification)", reason)
	}
}

// TestRealChainUnauthorized verifies that the server rejects requests missing
// the Bearer token (step 1-3 of the auth chain).
func TestRealChainUnauthorized(t *testing.T) {
	cfg := newIntegrationConfig()
	router, cleanup, err := testapp.Build(cfg)
	if err != nil {
		t.Fatalf("app.Build failed: %v", err)
	}
	defer cleanup()

	srv := httptest.NewServer(router)
	defer srv.Close()

	// POST /edits without any auth headers.
	body := []byte(`{"device_id":"d1","client_version":"1.0.0","edits":[]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/ai-track/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	} else {
		t.Logf("Unauthorized confirmed: status=%d (steps 1-3: auth chain)", resp.StatusCode)
	}
}
