package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/aievents"
	"github.com/cristianadrielbraun/gofer/internal/auth"
	"github.com/cristianadrielbraun/gofer/internal/llm"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func aiRequest(method, target, user, body string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: user}))
}

func newAIHandler(t *testing.T) (*Handler, *storage.DB) {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Write().Exec(`
		INSERT INTO users (id, username, username_normalized, name) VALUES
			('owner', 'owner', 'owner', 'Owner'),
			('other', 'other', 'other', 'Other')`); err != nil {
		t.Fatal(err)
	}
	return &Handler{db: db, aiWorkerReady: true}, db
}

func TestAISettingsCannotEnableAnalysisWithoutAWorker(t *testing.T) {
	h, _ := newAIHandler(t)
	h.SetAIWorkerReady(false)
	w := httptest.NewRecorder()
	h.handleAISettingsUpdate(w, aiRequest(http.MethodPut, "/api/ai/settings", "owner", `{"analysis_enabled":true}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s; want 409", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.handleAISettingsUpdate(w, aiRequest(http.MethodPut, "/api/ai/settings", "owner", `{"drafting_enabled":true}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("drafting without LLM status = %d, body %s; want 409", w.Code, w.Body.String())
	}
	client, err := llm.NewClient(llm.Config{BaseURL: "http://localhost:11434", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	h.SetAIDraftingClient(client)
	w = httptest.NewRecorder()
	h.handleAISettingsUpdate(w, aiRequest(http.MethodPut, "/api/ai/settings", "owner", `{"drafting_enabled":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("drafting-only status = %d, body %s", w.Code, w.Body.String())
	}
}

func TestAISettingsAPIStartsDisabledAndPersists(t *testing.T) {
	h, db := newAIHandler(t)
	client, err := llm.NewClient(llm.Config{BaseURL: "http://localhost:11434", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	h.SetAIDraftingClient(client)
	w := httptest.NewRecorder()
	h.handleAISettingsGet(w, aiRequest(http.MethodGet, "/api/ai/settings", "owner", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body %s", w.Code, w.Body.String())
	}
	var initial aiSettingsPayload
	if err := json.Unmarshal(w.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial != (aiSettingsPayload{}) {
		t.Fatalf("initial settings = %+v, want disabled", initial)
	}

	w = httptest.NewRecorder()
	h.handleAISettingsUpdate(w, aiRequest(http.MethodPut, "/api/ai/settings", "owner",
		`{"analysis_enabled":true,"drafting_enabled":true,"auto_spam_enabled":false,"auto_unsubscribe_enabled":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.handleAISettingsGet(w, aiRequest(http.MethodGet, "/api/ai/settings", "owner", ""))
	var stored aiSettingsPayload
	if err := json.Unmarshal(w.Body.Bytes(), &stored); err != nil {
		t.Fatal(err)
	}
	if !stored.AnalysisEnabled || !stored.DraftingEnabled || stored.AutoSpamEnabled || stored.AutoUnsubscribeEnabled {
		t.Fatalf("stored settings = %+v", stored)
	}
	events, err := aiaudit.List(t.Context(), db, "owner", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "ai.settings.update" {
		t.Fatalf("audit events = %+v", events)
	}

	w = httptest.NewRecorder()
	h.handleAISettingsGet(w, aiRequest(http.MethodGet, "/api/ai/settings", "other", ""))
	var other aiSettingsPayload
	if err := json.Unmarshal(w.Body.Bytes(), &other); err != nil {
		t.Fatal(err)
	}
	if other != (aiSettingsPayload{}) {
		t.Fatalf("other user settings = %+v", other)
	}
}

func TestAISettingsAPIRejectsUnknownFields(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()
	h.handleAISettingsUpdate(w, aiRequest(http.MethodPut, "/api/ai/settings", "owner", `{"analysis_enabled":true,"surprise":true}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAIAuditAPIListsCurrentUserEvents(t *testing.T) {
	h, db := newAIHandler(t)
	if _, err := aiaudit.Record(t.Context(), db, aiaudit.Event{UserID: "owner", Action: "analysis.complete"}); err != nil {
		t.Fatal(err)
	}
	if _, err := aiaudit.Record(t.Context(), db, aiaudit.Event{UserID: "other", Action: "analysis.complete"}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handleAIAuditList(w, aiRequest(http.MethodGet, "/api/ai/audit?limit=10", "owner", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var payload struct {
		Events []aiaudit.Event `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Events) != 1 || payload.Events[0].UserID != "owner" {
		t.Fatalf("events = %+v", payload.Events)
	}
	w = httptest.NewRecorder()
	h.handleAIAuditList(w, aiRequest(http.MethodGet, "/api/ai/audit?limit=nope", "owner", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want 400", w.Code)
	}
}

func TestAIEventsStreamIsAuthenticatedAndUserScoped(t *testing.T) {
	bus := aievents.NewBus()
	h := &Handler{aiEvents: bus}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(auth.ContextWithUser(r.Context(), &auth.User{ID: "owner"}))
		h.handleAIEvents(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/ai/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream response = %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(resp.Body)
	ready := false
	for scanner.Scan() {
		if scanner.Text() == "event: ready" {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("ready event missing")
	}
	var event, data string
	bus.Publish(aievents.Event{Type: "analysis.job", UserID: "owner", MessageID: 7, State: "done"})
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
			data = ""
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
		if event != "" && data != "" {
			break
		}
	}
	if event != "analysis.job" || !strings.Contains(data, `"message_id":7`) {
		t.Fatalf("stream event = %q %q", event, data)
	}
}
