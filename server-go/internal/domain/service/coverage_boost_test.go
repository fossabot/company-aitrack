package service_test

// coverage_boost_test.go covers previously-zero and low coverage paths in:
//   - profile_service.go (NewProfileService, BuildProfile)
//   - token_key_service.go (ComputeTokenKey, NewRawToken, RandomHex)
//   - keyword_service.go classifyPrompt (refactor, explain branches)
//   - validation_service.go isPathMismatch (control-char branch)
//   - encryptor.go (short ciphertext error path)

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/aitrack/server/internal/domain/service"
	"github.com/aitrack/server/internal/testkit"
)

// ─── profile_service.go ───────────────────────────────────────────────────────

func TestNewProfileService(t *testing.T) {
	svc := service.NewProfileService()
	if svc == nil {
		t.Error("NewProfileService() returned nil")
	}
}

func TestBuildProfile_EmptyRecords(t *testing.T) {
	svc := service.NewProfileService()
	now := time.Now()
	dto := svc.BuildProfile("tok123", "alice", nil, now)
	if dto == nil {
		t.Fatal("BuildProfile returned nil")
	}
	if dto.TokenKey != "tok123" {
		t.Errorf("TokenKey = %q, want %q", dto.TokenKey, "tok123")
	}
	if dto.Owner != "alice" {
		t.Errorf("Owner = %q, want %q", dto.Owner, "alice")
	}
	if dto.TotalEdits != 0 {
		t.Errorf("TotalEdits = %d, want 0", dto.TotalEdits)
	}
	if dto.Frequency == nil {
		t.Error("Frequency should not be nil for empty records")
	}
	if dto.Frequency.DailyTrend == nil {
		t.Error("DailyTrend should not be nil")
	}
	if dto.Depth == nil {
		t.Error("Depth should not be nil")
	}
	if dto.PromptPatterns == nil {
		t.Error("PromptPatterns should not be nil")
	}
}

func TestBuildProfile_SingleRecord(t *testing.T) {
	svc := service.NewProfileService()
	now := time.Unix(1716000000, 0).UTC()

	ts := "1716000000"
	ps := "fix the bug"
	records := []service.RawRecord{
		{
			Tool:          "claude",
			FilePath:      "src/main.go",
			AddedLines:    5,
			RemovedLines:  2,
			DiffHunk:      "+foo()\n+// comment\n-bar()",
			Timestamp:     ts,
			Status:        "ACCEPTED",
			PromptSummary: &ps,
		},
	}

	dto := svc.BuildProfile("tok-abc", "bob", records, now)

	if dto.TotalEdits != 1 {
		t.Errorf("TotalEdits = %d, want 1", dto.TotalEdits)
	}
	if dto.TotalAddedLines != 5 {
		t.Errorf("TotalAddedLines = %d, want 5", dto.TotalAddedLines)
	}
	if dto.TotalRemovedLines != 2 {
		t.Errorf("TotalRemovedLines = %d, want 2", dto.TotalRemovedLines)
	}
	if _, ok := dto.Tools["claude"]; !ok {
		t.Error("Tools should contain 'claude'")
	}
	if _, ok := dto.Languages["Go"]; !ok {
		t.Error("Languages should contain 'Go'")
	}
	if dto.FirstSeen == nil {
		t.Error("FirstSeen should be set")
	}
	if dto.LastSeen == nil {
		t.Error("LastSeen should be set")
	}
	if dto.Frequency == nil {
		t.Error("Frequency should not be nil")
	}
	if dto.Depth == nil {
		t.Error("Depth should not be nil")
	}
	if dto.PromptPatterns["fix_debug"] != 1 {
		t.Errorf("PromptPatterns[fix_debug] = %d, want 1", dto.PromptPatterns["fix_debug"])
	}
}

func TestBuildProfile_MultipleRecords_SortedDailyTrend(t *testing.T) {
	svc := service.NewProfileService()
	now := time.Unix(1716259200, 0).UTC()

	ts1 := "1716172800"
	ts2 := "1716086400"

	records := []service.RawRecord{
		{Tool: "cursor", FilePath: "main.py", AddedLines: 10, RemovedLines: 3, Timestamp: ts1, Status: "ACCEPTED"},
		{Tool: "claude", FilePath: "app.ts", AddedLines: 20, RemovedLines: 8, Timestamp: ts2, Status: "FLAGGED"},
	}

	dto := svc.BuildProfile("tok-multi", "charlie", records, now)

	if dto.TotalEdits != 2 {
		t.Errorf("TotalEdits = %d, want 2", dto.TotalEdits)
	}
	if dto.Frequency == nil {
		t.Fatal("Frequency is nil")
	}
	trend := dto.Frequency.DailyTrend
	for i := 1; i < len(trend); i++ {
		if trend[i].Date < trend[i-1].Date {
			t.Errorf("DailyTrend not sorted: trend[%d]=%q < trend[%d]=%q",
				i, trend[i].Date, i-1, trend[i-1].Date)
		}
	}
	if dto.Depth == nil {
		t.Fatal("Depth is nil")
	}
	total := dto.Depth.SmallCount + dto.Depth.MediumCount + dto.Depth.LargeCount
	if total != int64(len(records)) {
		t.Errorf("size bucket total = %d, want %d", total, len(records))
	}
}

func TestBuildProfile_InvalidTimestamp_Skipped(t *testing.T) {
	svc := service.NewProfileService()
	now := time.Now()

	records := []service.RawRecord{
		{Tool: "claude", FilePath: "src/main.rs", AddedLines: 1, RemovedLines: 0,
			Timestamp: "not-a-number", Status: "ACCEPTED"},
	}

	dto := svc.BuildProfile("tok-x", "dave", records, now)
	if dto.TotalEdits != 1 {
		t.Errorf("TotalEdits = %d, want 1", dto.TotalEdits)
	}
	if dto.FirstSeen != nil {
		t.Error("FirstSeen should be nil when timestamp is unparseable")
	}
}

func TestBuildProfile_SizeCategories(t *testing.T) {
	svc := service.NewProfileService()
	now := time.Now()

	ts := "1716000000"
	records := []service.RawRecord{
		{Tool: "claude", FilePath: "a.go", AddedLines: 2, RemovedLines: 1, Timestamp: ts, Status: "ACCEPTED"},   // small <10
		{Tool: "claude", FilePath: "b.go", AddedLines: 50, RemovedLines: 10, Timestamp: ts, Status: "ACCEPTED"}, // medium <=100
		{Tool: "claude", FilePath: "c.go", AddedLines: 200, RemovedLines: 0, Timestamp: ts, Status: "ACCEPTED"}, // large >100
	}

	dto := svc.BuildProfile("tok-sz", "eve", records, now)
	if dto.Depth.SmallCount != 1 {
		t.Errorf("SmallCount = %d, want 1", dto.Depth.SmallCount)
	}
	if dto.Depth.MediumCount != 1 {
		t.Errorf("MediumCount = %d, want 1", dto.Depth.MediumCount)
	}
	if dto.Depth.LargeCount != 1 {
		t.Errorf("LargeCount = %d, want 1", dto.Depth.LargeCount)
	}
}

func TestBuildProfile_FrequencyStats(t *testing.T) {
	svc := service.NewProfileService()
	// Set now to a fixed time so timestamps within 30d and 12w windows work correctly
	now := time.Unix(1716259200, 0).UTC()

	// Timestamp within last 30 days and 12 weeks
	recent := "1716172800"
	records := []service.RawRecord{
		{Tool: "claude", FilePath: "x.go", AddedLines: 3, RemovedLines: 1, Timestamp: recent, Status: "ACCEPTED"},
		{Tool: "claude", FilePath: "y.go", AddedLines: 3, RemovedLines: 1, Timestamp: recent, Status: "ACCEPTED"},
	}

	dto := svc.BuildProfile("tok-freq", "frank", records, now)
	if dto.Frequency.DailyAvg30d <= 0 {
		t.Errorf("DailyAvg30d = %f, want > 0", dto.Frequency.DailyAvg30d)
	}
	if dto.Frequency.WeeklyAvg12w <= 0 {
		t.Errorf("WeeklyAvg12w = %f, want > 0", dto.Frequency.WeeklyAvg12w)
	}
}

// ─── token_key_service.go ─────────────────────────────────────────────────────

func TestComputeTokenKey_WithPrefix(t *testing.T) {
	// "aitrack_abcdef1234567890xy" → stripped = "abcdef1234567890xy" (18 chars > 10)
	// → first 6 = "abcdef", last 4 = "90xy"
	got := service.ComputeTokenKey("aitrack_abcdef1234567890xy")
	if !strings.Contains(got, "…") {
		t.Errorf("expected ellipsis in %q", got)
	}
	if !strings.HasPrefix(got, "abcdef") {
		t.Errorf("expected prefix 'abcdef', got %q", got)
	}
	if !strings.HasSuffix(got, "90xy") {
		t.Errorf("expected suffix '90xy', got %q", got)
	}
}

func TestComputeTokenKey_ShortNoEllipsis(t *testing.T) {
	got := service.ComputeTokenKey("aitrack_short")
	if strings.Contains(got, "…") {
		t.Errorf("short token should not have ellipsis: %q", got)
	}
	if got != "short" {
		t.Errorf("expected 'short', got %q", got)
	}
}

func TestComputeTokenKey_NoPrefix(t *testing.T) {
	raw := "rawtoken1234567890"
	got := service.ComputeTokenKey(raw)
	if !strings.Contains(got, "…") {
		t.Errorf("long token without prefix should have ellipsis: %q", got)
	}
	if !strings.HasPrefix(got, "rawtok") {
		t.Errorf("expected prefix 'rawtok', got %q", got)
	}
	if !strings.HasSuffix(got, "7890") {
		t.Errorf("expected suffix '7890', got %q", got)
	}
}

func TestComputeTokenKey_ExactlyTenChars(t *testing.T) {
	// After stripping prefix: exactly 10 chars → return as-is (no ellipsis)
	got := service.ComputeTokenKey("aitrack_0123456789")
	if strings.Contains(got, "…") {
		t.Errorf("exactly 10 chars should not have ellipsis: %q", got)
	}
	if got != "0123456789" {
		t.Errorf("expected '0123456789', got %q", got)
	}
}

func TestNewRawToken_Format(t *testing.T) {
	tok := service.NewRawToken()
	if !strings.HasPrefix(tok, "aitrack_") {
		t.Errorf("NewRawToken() = %q, must start with 'aitrack_'", tok)
	}
	// "aitrack_" (8 chars) + 64 hex chars = 72 total
	if len(tok) != 72 {
		t.Errorf("NewRawToken() length = %d, want 72", len(tok))
	}
}

func TestNewRawToken_Unique(t *testing.T) {
	t1 := service.NewRawToken()
	t2 := service.NewRawToken()
	if t1 == t2 {
		t.Error("NewRawToken() should return unique values")
	}
}

func TestRandomHex_Length(t *testing.T) {
	h := service.RandomHex(16)
	if len(h) != 32 {
		t.Errorf("RandomHex(16) length = %d, want 32", len(h))
	}
}

func TestRandomHex_Unique(t *testing.T) {
	h1 := service.RandomHex(32)
	h2 := service.RandomHex(32)
	if h1 == h2 {
		t.Error("RandomHex should return unique values")
	}
}

// ─── keyword_service.go — missing classifyPrompt branches ────────────────────

func TestComputePromptPatterns_RefactorKeyword(t *testing.T) {
	ps := "refactor this module"
	records := []service.RawRecord{{PromptSummary: &ps}}
	patterns := service.ComputePromptPatterns(records)
	if patterns["refactor"] != 1 {
		t.Errorf("refactor = %d, want 1", patterns["refactor"])
	}
}

func TestComputePromptPatterns_ExplainKeyword(t *testing.T) {
	ps := "explain how this works"
	records := []service.RawRecord{{PromptSummary: &ps}}
	patterns := service.ComputePromptPatterns(records)
	if patterns["explain"] != 1 {
		t.Errorf("explain = %d, want 1", patterns["explain"])
	}
}

func TestComputePromptPatterns_TestKeyword(t *testing.T) {
	ps := "write unit tests"
	records := []service.RawRecord{{PromptSummary: &ps}}
	patterns := service.ComputePromptPatterns(records)
	if patterns["test"] != 1 {
		t.Errorf("test = %d, want 1", patterns["test"])
	}
}

func TestComputePromptPatterns_EmptyPromptSkipped(t *testing.T) {
	empty := ""
	records := []service.RawRecord{{PromptSummary: &empty}}
	patterns := service.ComputePromptPatterns(records)
	for k, v := range patterns {
		if v != 0 {
			t.Errorf("empty prompt: patterns[%q] = %d, want 0", k, v)
		}
	}
}

// ─── validation_service.go — isPathMismatch control-char branch ──────────────

func TestValidation_PathMismatch_ControlChar_Flagged(t *testing.T) {
	svc := defaultValidationSvc(&fakeCounter{0})
	token := testkit.BuildTokenWithSig()
	// NUL byte (0x00) in file_path should trigger control-char flag
	edit := testkit.BuildEditDTO(func(p *testkit.EditParams) {
		p.FilePath = "src/\x00file.go"
		p.RepoURL = "git@github.com:org/repo.git"
	})

	result := svc.Validate(token, edit)
	if !containsReason(result.Reasons, "path_mismatch") {
		t.Errorf("control-char in file_path should flag path_mismatch, got %v", result.Reasons)
	}
}

func TestValidation_PathMismatch_TabChar_Flagged(t *testing.T) {
	svc := defaultValidationSvc(&fakeCounter{0})
	token := testkit.BuildTokenWithSig()
	edit := testkit.BuildEditDTO(func(p *testkit.EditParams) {
		p.FilePath = "src/\x09file.go" // 0x09 = tab, which is <0x20
		p.RepoURL = "git@github.com:org/repo.git"
	})

	result := svc.Validate(token, edit)
	if !containsReason(result.Reasons, "path_mismatch") {
		t.Errorf("tab char in file_path should flag path_mismatch, got %v", result.Reasons)
	}
}

// ─── encryptor.go missing error paths ─────────────────────────────────────────

func TestEncryptorDecryptTooShort(t *testing.T) {
	key := make([]byte, 32)
	b64Key := base64.StdEncoding.EncodeToString(key)
	enc, err := service.NewHmacSecretEncryptor(b64Key)
	if err != nil {
		t.Fatal(err)
	}
	// A valid base64 that decodes to fewer than 12 bytes (< ivBytes)
	short := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	_, err = enc.Decrypt(short)
	if err == nil {
		t.Error("expected error for ciphertext shorter than IV length")
	}
}

func TestEncryptorDecryptBadBase64(t *testing.T) {
	key := make([]byte, 32)
	b64Key := base64.StdEncoding.EncodeToString(key)
	enc, err := service.NewHmacSecretEncryptor(b64Key)
	if err != nil {
		t.Fatal(err)
	}
	_, err = enc.Decrypt("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64 ciphertext")
	}
}

func TestEncryptorDecryptGCMFail(t *testing.T) {
	key := make([]byte, 32)
	b64Key := base64.StdEncoding.EncodeToString(key)
	enc, err := service.NewHmacSecretEncryptor(b64Key)
	if err != nil {
		t.Fatal(err)
	}
	// Manufacture a fake ciphertext: valid base64, ≥ 12 bytes but GCM auth tag will fail
	fakeRaw := make([]byte, 28) // 12-byte IV + 16 bytes (minimum GCM tag), all zeros
	fakeB64 := base64.StdEncoding.EncodeToString(fakeRaw)
	_, err = enc.Decrypt(fakeB64)
	if err == nil {
		t.Error("expected GCM authentication error for tampered ciphertext")
	}
}
