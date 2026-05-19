// coverage_test.go covers remaining uncovered branches across the
// application use-case layer (token, ingest).
package application_test

import (
	"strings"
	"testing"

	dbadapter "github.com/aitrack/server/internal/adapter/db"
	"github.com/aitrack/server/internal/application"
	"github.com/aitrack/server/internal/domain/model"
	"github.com/aitrack/server/internal/domain/service"
	"github.com/aitrack/server/internal/testkit"
)

// ── Token boolToInt / inactive branch ────────────────────────────────────────

func TestBoolToInt_ViaActiveToken(t *testing.T) {
	// boolToInt(false) is exercised when active=false is stored.
	// We test it indirectly by creating an inactive token and checking it returns nil.
	database := openTestDB(t)
	enc, _ := service.NewHmacSecretEncryptor("")
	sig := service.NewSignatureService()
	repo := dbadapter.NewTokenAdapter(database)
	svc := application.NewTokenService(repo, sig, enc)

	resp, _ := svc.CreateToken(&model.CreateTokenRequest{Owner: "test"})
	// Force active=false
	database.Exec("UPDATE tokens SET active=0")
	rawToken := strings.SplitN(resp.Credential, "-", 2)[0]
	found, err := svc.FindActiveToken(rawToken)
	if err != nil {
		t.Fatal(err)
	}
	if found != nil {
		t.Error("inactive token should return nil")
	}
}

// ── Ingest saveEdit with optional fields ─────────────────────────────────────

func TestIngest_SaveEditWithAllOptionalFields(t *testing.T) {
	database := openTestDB(t)
	policy := service.ValidationPolicy{RateLimitPerHour: 30, MaxAddedLines: 5000}

	sig := service.NewSignatureService()
	diff := service.NewDiffConsistencyService()
	editRepo := dbadapter.NewEditRecordAdapter(database)
	validation := service.NewValidationService(sig, diff, editRepo, policy)
	validator := service.NewEditValidator()
	ingest := application.NewIngestService(validation, validator, editRepo)

	token := testkit.BuildTokenWithSig()

	// Build edit with model, diffhunk, and metadata set
	modelStr := "claude-3-5-sonnet"
	meta := `{"key":"value"}`
	diff_ := "@@ -1 +1 @@\n-old\n+new\n"
	edit := testkit.BuildEditDTO(func(p *testkit.EditParams) {
		p.DiffHunk = &diff_
		p.AddedLines = 1
		p.RemovedLines = 1
	})
	edit.Model = &modelStr
	edit.Metadata = &meta

	req := &model.EditBatchRequest{
		DeviceID: "dev-001",
		Edits:    []model.EditDTO{*edit},
	}
	resp := ingest.Ingest(token, req)
	if resp.Accepted != 1 {
		t.Errorf("expected 1 accepted, got %d; rejected=%v flagged=%v",
			resp.Accepted, resp.Rejected, resp.Flagged)
	}
}

// ── Ingest QueryEdits with no results ────────────────────────────────────────

func TestIngest_QueryEdits_Empty(t *testing.T) {
	database := openTestDB(t)
	policy := service.ValidationPolicy{RateLimitPerHour: 30, MaxAddedLines: 5000}
	sig := service.NewSignatureService()
	diff := service.NewDiffConsistencyService()
	editRepo := dbadapter.NewEditRecordAdapter(database)
	validation := service.NewValidationService(sig, diff, editRepo, policy)
	validator := service.NewEditValidator()
	ingest := application.NewIngestService(validation, validator, editRepo)

	result, err := ingest.QueryEdits("no-such-token", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0, got %d", result.Total)
	}
	if result.Records == nil {
		t.Error("Records should be non-nil empty slice")
	}
}
