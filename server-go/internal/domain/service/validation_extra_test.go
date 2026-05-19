package service_test

import (
	"testing"

	"github.com/aitrack/server/internal/domain/service"
	"github.com/aitrack/server/internal/testkit"
)

// ── EditValidator nil edit ────────────────────────────────────────────────────

func TestEditValidator_NilEdit(t *testing.T) {
	v := service.NewEditValidator()
	if reason := v.Validate(nil); reason != "malformed" {
		t.Errorf("nil edit: expected malformed, got %q", reason)
	}
}

// ── isPathMismatch edge cases ─────────────────────────────────────────────────

func TestValidation_PathMismatch_LocalRepo(t *testing.T) {
	// Absolute path with local repo URL → not a mismatch
	svc := defaultValidationSvc(&fakeCounter{0})
	token := testkit.BuildTokenWithSig()
	edit := testkit.BuildEditDTO(func(p *testkit.EditParams) {
		p.FilePath = "/home/user/project/src/main.rs"
		p.RepoURL = "/home/user/project"
	})
	result := svc.Validate(token, edit)
	for _, r := range result.Reasons {
		if r == "path_mismatch" {
			t.Error("local repo with absolute path should not flag path_mismatch")
		}
	}
}

func TestValidation_PathMismatch_FileScheme(t *testing.T) {
	svc := defaultValidationSvc(&fakeCounter{0})
	token := testkit.BuildTokenWithSig()
	edit := testkit.BuildEditDTO(func(p *testkit.EditParams) {
		p.FilePath = "/home/user/project/src/main.rs"
		p.RepoURL = "file:///home/user/project"
	})
	result := svc.Validate(token, edit)
	for _, r := range result.Reasons {
		if r == "path_mismatch" {
			t.Error("file:// repo with absolute path should not flag path_mismatch")
		}
	}
}

func TestValidation_PathMismatch_EmptyFields(t *testing.T) {
	svc := defaultValidationSvc(&fakeCounter{0})
	token := testkit.BuildTokenWithSig()
	edit := testkit.BuildEditDTO(func(p *testkit.EditParams) {
		p.FilePath = "src/main.rs"
		p.RepoURL = "git@github.com:org/repo.git"
	})
	result := svc.Validate(token, edit)
	for _, r := range result.Reasons {
		if r == "path_mismatch" {
			t.Error("relative path should never flag path_mismatch")
		}
	}
}

// ── containsStr (whitelist present, URL in list) ──────────────────────────────

func TestValidation_RepoInWhitelist_NoFlag(t *testing.T) {
	policy := service.ValidationPolicy{
		RateLimitPerHour:  30,
		MaxAddedLines:     5000,
		EnforceWhitelist:  true,
		RepoWhitelistURLs: []string{"git@github.com:org/repo.git"},
	}
	svc := service.NewValidationService(
		service.NewSignatureService(),
		service.NewDiffConsistencyService(),
		&fakeCounter{0},
		policy,
	)
	token := testkit.BuildTokenWithSig()
	edit := testkit.BuildEditDTO() // repoURL = "git@github.com:org/repo.git" (in whitelist)

	result := svc.Validate(token, edit)
	if result.Outcome == service.OutcomeRejected {
		t.Errorf("repo in whitelist should not be rejected, got %v", result.Reasons)
	}
}
