package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func TestAIAPITokensCreateAuthenticateAndRevoke(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()
	h.handleAIAPITokens(w, aiRequest(http.MethodPost, "/api/ai/tokens", "owner", `{"name":"Shortcut","scopes":["read"],"expires_in_days":30}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", w.Code, w.Body.String())
	}
	var created struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Token, "gfr_") || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}
	w = httptest.NewRecorder()
	h.handleAIAPITokens(w, aiRequest(http.MethodGet, "/api/ai/tokens", "owner", ``))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), created.Token) || strings.Contains(w.Body.String(), "token_hash") {
		t.Fatalf("list status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+created.Token)
	h.RequireAIToken("read", http.HandlerFunc(h.handleAPIv1Me)).ServeHTTP(w, request)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"user_id":"owner"`) {
		t.Fatalf("me status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+created.Token)
	h.RequireAIToken("write", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, request)
	if w.Code != http.StatusForbidden {
		t.Fatalf("write scope status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer gfr_not-a-token")
	h.RequireAIToken("read", http.HandlerFunc(h.handleAPIv1Me)).ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	request = aiRequest(http.MethodDelete, "/api/ai/tokens/"+created.ID, "owner", ``)
	request.SetPathValue("id", created.ID)
	h.handleAIAPIToken(w, request)
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+created.Token)
	h.RequireAIToken("read", http.HandlerFunc(h.handleAPIv1Me)).ServeHTTP(w, request)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d", w.Code)
	}
}

func TestAPITokenRateLimitRefills(t *testing.T) {
	h := &Handler{}
	now := time.Now()
	for i := 0; i < 30; i++ {
		if !h.allowAPITokenRequest("token", now) {
			t.Fatalf("request %d was limited", i)
		}
	}
	if h.allowAPITokenRequest("token", now) {
		t.Fatal("request above the burst was allowed")
	}
	if !h.allowAPITokenRequest("token", now.Add(time.Second)) {
		t.Fatal("request after refill was limited")
	}
}

func TestAIAPITokensRejectUnknownScopesAndForeignRevoke(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()
	h.handleAIAPITokens(w, aiRequest(http.MethodPost, "/api/ai/tokens", "owner", `{"name":"Bad","scopes":["admin"]}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown scope status = %d, body %s", w.Code, w.Body.String())
	}
	created, err := h.db.CreateAIAPIToken(t.Context(), storage.AIAPIToken{
		UserID: "owner", Name: "Foreign", TokenHash: storage.HashAIAPIToken("gfr_foreign"), Scopes: "read",
	})
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	request := aiRequest(http.MethodDelete, "/api/ai/tokens/"+created.ID, "other", ``)
	request.SetPathValue("id", created.ID)
	h.handleAIAPIToken(w, request)
	if w.Code != http.StatusNotFound {
		t.Fatalf("foreign revoke status = %d, body %s", w.Code, w.Body.String())
	}
}
