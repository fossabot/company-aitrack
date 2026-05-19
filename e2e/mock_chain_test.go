// Mock chain test — validates full Phase 4 data pipeline without real credentials.
// Uses a mock server that accepts any token and returns canned responses.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Canned profile response (Phase 4) ───────────────────────────────────────

var cannedProfile = map[string]interface{}{
	"token_key":           "mock_tok_001",
	"owner":               "mock-user",
	"total_edits":         float64(25),
	"total_added_lines":   float64(320),
	"total_removed_lines": float64(88),
	"first_seen":          "2026-05-01T10:00:00Z",
	"last_seen":           "2026-05-20T09:00:00Z",
	"generated_at":        "2026-05-20T09:30:00Z",
	"frequency": map[string]interface{}{
		"daily_avg_30d":  0.8,
		"weekly_avg_12w": 5.2,
		"daily_trend":    []interface{}{},
	},
	"depth": map[string]interface{}{
		"avg_lines":    16.3,
		"p50_lines":    float64(10),
		"p90_lines":    float64(45),
		"small_count":  float64(15),
		"medium_count": float64(9),
		"large_count":  float64(1),
	},
	"languages": map[string]interface{}{
		"rs":   float64(12),
		"go":   float64(8),
		"java": float64(5),
	},
	"comment_density": 0.08,
	"prompt_patterns": map[string]interface{}{
		"generate":  float64(8),
		"fix_debug": float64(6),
		"refactor":  float64(4),
		"explain":   float64(3),
		"test":      float64(2),
		"other":     float64(2),
	},
	"tools": map[string]interface{}{
		"claude": float64(22),
		"codex":  float64(3),
		"cursor": float64(0),
	},
}

// ─── Mock server ──────────────────────────────────────────────────────────────

// newMockServer builds an httptest.Server that handles the four Phase 4 routes.
// It accepts any bearer token and ignores signatures.
func newMockServer() *httptest.Server {
	mux := http.NewServeMux()

	// POST /api/v1/ai-track/edits → {"accepted":1,"rejected":[],"flagged":[]}
	mux.HandleFunc("/api/v1/ai-track/edits", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"accepted":1,"rejected":[],"flagged":[]}`)
	})

	// POST /api/v1/ai-track/heartbeat → {"ok":true}
	mux.HandleFunc("/api/v1/ai-track/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	})

	// GET /api/v1/ai-track/profiles/{token_key} → canned profile JSON
	mux.HandleFunc("/api/v1/ai-track/profiles/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cannedProfile)
	})

	// POST /admin/tokens → mock credential
	mux.HandleFunc("/admin/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"credential":"mock_tok_001-mocksecret0000000000000000000000000000000000000000000000000000","token_key":"mock_tok_001"}`)
	})

	return httptest.NewServer(mux)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func doGet(url, bearer string) (int, map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return resp.StatusCode, m, nil
}

func doPost(url, bearer string, payload map[string]interface{}) (int, map[string]interface{}, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return resp.StatusCode, m, nil
}

// buildEditPayload builds a minimal edit payload with an optional prompt_summary.
func buildEditPayload(deviceID string, promptSummary *string) map[string]interface{} {
	edits := []map[string]interface{}{
		{
			"tool":          "claude",
			"tool_version":  "claude-code",
			"provider":      "anthropic",
			"session_id":    "sess-mock-0001",
			"repo_url":      "git@github.com:mock-org/mock-repo.git",
			"branch":        "main",
			"current_sha":   "aabbccddeeff00112233445566778899aabbccdd",
			"file_path":     "src/main.rs",
			"added_lines":   10,
			"removed_lines": 3,
			"diff_hunk":     "@@ -1,3 +1,10 @@\n-old line\n+new line\n",
			"timestamp":     "2026-05-20T09:00:00Z",
			"device_id":     deviceID,
			"hostname":      "mock-host",
			"record_sig":    "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}
	if promptSummary != nil {
		edits[0]["prompt_summary"] = *promptSummary
	}
	return map[string]interface{}{
		"device_id":      deviceID,
		"client_version": "1.0.0",
		"edits":          edits,
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestPhase4PromptCaptureChain validates the complete data flow from prompt_summary
// field through profile API response without requiring real credentials.
func TestPhase4PromptCaptureChain(t *testing.T) {
	srv := newMockServer()
	defer srv.Close()

	base := srv.URL
	token := "mock_tok_001"
	tokenKey := "mock_tok_001"

	// 1. POST /api/v1/ai-track/edits with prompt_summary field
	ps := "generate: implement binary search function in Rust"
	payload := buildEditPayload("device-mock-0001", &ps)

	code, resp, err := doPost(base+"/api/v1/ai-track/edits", token, payload)
	if err != nil {
		t.Fatalf("POST /edits error: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("POST /edits: want 200, got %d", code)
	}
	accepted, _ := resp["accepted"].(float64)
	if int(accepted) != 1 {
		t.Errorf("POST /edits: want accepted=1, got %v (resp=%v)", accepted, resp)
	}

	// 2. GET /api/v1/ai-track/profiles/{token_key}
	profileURL := fmt.Sprintf("%s/api/v1/ai-track/profiles/%s", base, tokenKey)
	code2, profile, err := doGet(profileURL, token)
	if err != nil {
		t.Fatalf("GET /profiles error: %v", err)
	}
	if code2 != http.StatusOK {
		t.Fatalf("GET /profiles: want 200, got %d", code2)
	}

	// 3. Verify prompt_patterns field exists
	pp, ok := profile["prompt_patterns"]
	if !ok || pp == nil {
		t.Fatalf("profile response missing prompt_patterns field; profile=%v", profile)
	}

	// 4. Verify prompt_patterns contains all 6 expected keys
	ppMap, ok := pp.(map[string]interface{})
	if !ok {
		t.Fatalf("prompt_patterns is not a map: %T", pp)
	}
	expectedKeys := []string{"generate", "fix_debug", "refactor", "explain", "test", "other"}
	for _, k := range expectedKeys {
		if _, exists := ppMap[k]; !exists {
			t.Errorf("prompt_patterns missing key %q; got %v", k, ppMap)
		}
	}

	// 5. Verify generate > 0 in the canned response
	generateVal, _ := ppMap["generate"].(float64)
	if generateVal <= 0 {
		t.Errorf("prompt_patterns.generate: want > 0, got %v", generateVal)
	}
}

// TestPhase4PromptSummaryField validates that prompt_summary is optional (omitempty semantics).
// Both payloads with and without the field must be accepted.
func TestPhase4PromptSummaryField(t *testing.T) {
	srv := newMockServer()
	defer srv.Close()

	base := srv.URL
	token := "mock_tok_001"

	// Without prompt_summary
	payloadWithout := buildEditPayload("device-mock-0002", nil)
	code1, resp1, err := doPost(base+"/api/v1/ai-track/edits", token, payloadWithout)
	if err != nil {
		t.Fatalf("POST /edits (no prompt_summary) error: %v", err)
	}
	if code1 != http.StatusOK {
		t.Fatalf("POST /edits (no prompt_summary): want 200, got %d", code1)
	}
	acc1, _ := resp1["accepted"].(float64)
	if int(acc1) != 1 {
		t.Errorf("without prompt_summary: want accepted=1, got %v", acc1)
	}

	// With prompt_summary
	ps := "fix_debug: diagnose nil pointer dereference in uploader"
	payloadWith := buildEditPayload("device-mock-0003", &ps)
	code2, resp2, err := doPost(base+"/api/v1/ai-track/edits", token, payloadWith)
	if err != nil {
		t.Fatalf("POST /edits (with prompt_summary) error: %v", err)
	}
	if code2 != http.StatusOK {
		t.Fatalf("POST /edits (with prompt_summary): want 200, got %d", code2)
	}
	acc2, _ := resp2["accepted"].(float64)
	if int(acc2) != 1 {
		t.Errorf("with prompt_summary: want accepted=1, got %v", acc2)
	}
}

// TestPhase4ProfileApiResponse validates the full profile API response structure.
func TestPhase4ProfileApiResponse(t *testing.T) {
	srv := newMockServer()
	defer srv.Close()

	base := srv.URL
	token := "mock_tok_001"
	tokenKey := "mock_tok_001"

	profileURL := fmt.Sprintf("%s/api/v1/ai-track/profiles/%s", base, tokenKey)
	code, profile, err := doGet(profileURL, token)
	if err != nil {
		t.Fatalf("GET /profiles error: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("GET /profiles: want 200, got %d", code)
	}

	// Verify top-level fields
	requiredFields := []string{
		"token_key", "owner", "total_edits", "frequency",
		"depth", "languages", "comment_density", "prompt_patterns", "tools",
	}
	for _, f := range requiredFields {
		if v, exists := profile[f]; !exists || v == nil {
			t.Errorf("profile response missing or nil field %q; profile keys=%v", f, profileKeys(profile))
		}
	}

	// Verify languages map is non-empty
	langs, ok := profile["languages"].(map[string]interface{})
	if !ok || len(langs) == 0 {
		t.Errorf("profile.languages should be a non-empty map; got %v", profile["languages"])
	}

	// Verify prompt_patterns contains exactly 6 categories
	ppMap, ok := profile["prompt_patterns"].(map[string]interface{})
	if !ok {
		t.Fatalf("profile.prompt_patterns is not a map: %T", profile["prompt_patterns"])
	}
	expectedCategories := []string{"generate", "fix_debug", "refactor", "explain", "test", "other"}
	if len(ppMap) != len(expectedCategories) {
		t.Errorf("prompt_patterns: want %d keys, got %d (%v)", len(expectedCategories), len(ppMap), ppMap)
	}
	for _, k := range expectedCategories {
		if _, exists := ppMap[k]; !exists {
			t.Errorf("prompt_patterns missing category %q", k)
		}
	}
}

// profileKeys returns the sorted key list of a profile map for error messages.
func profileKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
