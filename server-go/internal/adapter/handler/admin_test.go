package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aitrack/server/internal/domain/model"
)

func TestAdmin_CreateToken_OK(t *testing.T) {
	env := newTestEnv(t)
	req := env.adminRequest(http.MethodPost, "/admin/tokens",
		model.CreateTokenRequest{Owner: "alice", Note: "test token"})

	w := do(env.router, req)
	assertStatus(t, w, http.StatusOK)

	var resp model.CreateTokenResponse
	decodeJSON(t, w, &resp)
	if resp.Credential == "" {
		t.Error("credential should not be empty")
	}
	if resp.TokenKey == "" {
		t.Error("token_key should not be empty")
	}
	// credential must be "<token>-<hmac_secret>"; token starts with "aitrack_"
	parts := strings.SplitN(resp.Credential, "-", 2)
	if len(parts) != 2 {
		t.Fatalf("credential %q does not contain '-'", resp.Credential)
	}
	if !strings.HasPrefix(parts[0], "aitrack_") {
		t.Errorf("credential token part %q should start with aitrack_", parts[0])
	}
	if parts[1] == "" {
		t.Error("credential hmac_secret part should not be empty")
	}
}

func TestAdmin_CreateToken_MissingAdminKey(t *testing.T) {
	env := newTestEnv(t)
	req := env.adminRequest(http.MethodPost, "/admin/tokens", model.CreateTokenRequest{Owner: "alice"})
	req.Header.Del("X-Admin-Key")

	w := do(env.router, req)
	assertStatus(t, w, http.StatusForbidden)
}

func TestAdmin_CreateToken_WrongAdminKey(t *testing.T) {
	env := newTestEnv(t)
	req := env.adminRequest(http.MethodPost, "/admin/tokens", model.CreateTokenRequest{Owner: "alice"})
	req.Header.Set("X-Admin-Key", "wrong-key")

	w := do(env.router, req)
	assertStatus(t, w, http.StatusForbidden)
}

func TestAdmin_CreateToken_NoAdminKeyConfigured(t *testing.T) {
	env := newTestEnv(t)
	env.cfg.AdminKey = ""
	req := env.adminRequest(http.MethodPost, "/admin/tokens", model.CreateTokenRequest{Owner: "alice"})

	w := do(env.router, req)
	assertStatus(t, w, http.StatusServiceUnavailable)
}

func TestAdmin_CreateToken_MissingOwner(t *testing.T) {
	env := newTestEnv(t)
	req := env.adminRequest(http.MethodPost, "/admin/tokens", model.CreateTokenRequest{Owner: ""})

	w := do(env.router, req)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdmin_CreateToken_InvalidJSON(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", env.cfg.AdminKey)

	w := do(env.router, req)
	assertStatus(t, w, http.StatusBadRequest)
}
