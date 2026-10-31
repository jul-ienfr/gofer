package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/config"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func TestAIConnectorsSealSecretsAndEnforceTransport(t *testing.T) {
	h, db := newAIHandler(t)
	store, err := config.NewAccountStore(db, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	h.accountStore = store

	w := httptest.NewRecorder()
	h.handleAIConnectors(w, aiRequest(http.MethodPost, "/api/ai/connectors", "owner",
		`{"name":"n8n","type":"n8n","url":"http://example.com/hook","secret":"s3cret","enabled":true}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("plain HTTP status = %d, body %s; want 400", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.handleAIConnectors(w, aiRequest(http.MethodPost, "/api/ai/connectors", "owner",
		`{"name":"n8n","type":"n8n","url":"https://example.com/hook","secret":"s3cret","enabled":true}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", w.Code, w.Body.String())
	}
	var created aiConnectorView
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.HasSecret || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}

	var encrypted string
	if err := db.Read().QueryRow(`SELECT secret_encrypted FROM ai_connectors WHERE id = ?`, created.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted == "" || strings.Contains(encrypted, "s3cret") {
		t.Fatal("connector secret was not encrypted")
	}
	secret, err := store.DecryptAIConnectorSecret("owner", created.ID, encrypted)
	if err != nil || secret != "s3cret" {
		t.Fatalf("decrypt = %q, %v", secret, err)
	}

	w = httptest.NewRecorder()
	h.handleAIConnectors(w, aiRequest(http.MethodGet, "/api/ai/connectors", "owner", ""))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "s3cret") || !strings.Contains(w.Body.String(), `"has_secret":true`) {
		t.Fatalf("list status = %d, body %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	patch := aiRequest(http.MethodPatch, "/api/ai/connectors/"+created.ID, "owner", `{"enabled":false}`)
	patch.SetPathValue("id", created.ID)
	h.handleAIConnector(w, patch)
	if w.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d, body %s", w.Code, w.Body.String())
	}
	var enabled bool
	if err := db.Read().QueryRow(`SELECT enabled FROM ai_connectors WHERE id = ?`, created.ID).Scan(&enabled); err != nil || enabled {
		t.Fatalf("enabled = %t, %v", enabled, err)
	}
}

func TestAIRoutingStatusIsEmptyAndOwnerScoped(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()
	h.handleAIRoutingStatus(w, aiRequest(http.MethodGet, "/api/ai/routing", "owner", ""))
	if w.Code != http.StatusOK || w.Body.String() != "{\"actions\":[],\"deliveries\":[],\"unsubscribes\":[]}\n" {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
}

func TestAIConnectorsAreOwnerScoped(t *testing.T) {
	h, _ := newAIHandler(t)
	connector := storage.AIConnector{
		ID: "connector", UserID: "owner", Name: "n8n", Type: "n8n",
		URL: "https://example.com/hook", SecretEncrypted: "enc", Enabled: true,
	}
	if _, err := h.db.CreateAIConnector(t.Context(), connector); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handleAIConnectors(w, aiRequest(http.MethodGet, "/api/ai/connectors", "other", ""))
	if w.Code != http.StatusOK || w.Body.String() != "{\"connectors\":[]}\n" {
		t.Fatalf("cross-user list status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	remove := aiRequest(http.MethodDelete, "/api/ai/connectors/connector", "other", "")
	remove.SetPathValue("id", "connector")
	h.handleAIConnector(w, remove)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-user delete status = %d, want 404", w.Code)
	}
}
