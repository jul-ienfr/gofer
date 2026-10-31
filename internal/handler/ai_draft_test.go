package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/llm"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func draftMessageRequest(method, user, id, body string) *http.Request {
	r := aiRequest(method, "/api/ai/messages/"+id+"/draft", user, body)
	r.SetPathValue("id", id)
	return r
}

func TestAIDraftReturnsTextWithoutCreatingOrSendingMail(t *testing.T) {
	h, db := newAIHandler(t)
	ctx := context.Background()
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, display_name, provider) VALUES ('acc', 'owner', 'owner@example.com', 'Owner', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email, preview_text) VALUES (1, 'acc', '<one@example.com>', 'Interview', 'alice@example.com', 'Can we meet Tuesday?');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAIAnalysis(ctx, storage.AIAnalysisResult{
		MessageID: 1, UserID: "owner", AccountID: "acc", AnalysisVersion: 1,
		Model: "jev", RiskLabel: "legit", RiskConfidence: 0.95,
		PriorityLabel: "high", IntentLabel: "reply_required", ProjectLabel: "employment", PromptVersion: "jev-v1",
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "owner", "ai_drafting_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "Can we meet Tuesday?") {
			t.Errorf("prompt does not include the message context: %s", raw)
		}
		_, _ = io.WriteString(w, `{"model":"fake","choices":[{"message":{"content":"{\"reply_text\":\"Mardi me convient.\",\"language\":\"fr\",\"confidence\":0.9,\"needs_human\":false,\"reason\":\"ok\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	}))
	defer server.Close()
	client, err := llm.NewClient(llm.Config{BaseURL: server.URL, Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	h.SetAIDraftingClient(client)
	w := httptest.NewRecorder()
	h.handleAIDraft(w, draftMessageRequest(http.MethodPost, "owner", "1", `{"instruction":"Réponds brièvement","tone":"professionnel"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["reply_text"] != "Mardi me convient." || payload["language"] != "fr" || payload["model"] != "fake" {
		t.Fatalf("payload = %+v", payload)
	}
	var drafts, generations, audits int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM messages WHERE from_email = 'owner@example.com'`).Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_llm_generations`).Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_audit_events WHERE action = 'ai.draft.generate'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if drafts != 0 || generations != 1 || audits != 1 {
		t.Fatalf("drafts=%d generations=%d audits=%d", drafts, generations, audits)
	}
	w = httptest.NewRecorder()
	h.handleAIDraft(w, draftMessageRequest(http.MethodPost, "other", "1", `{}`))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-user status = %d, body %s", w.Code, w.Body.String())
	}
}

func TestAIDraftHonorsSettingsAndSafety(t *testing.T) {
	h, db := newAIHandler(t)
	ctx := context.Background()
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'owner', 'owner@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"reply_text\":\"ok\",\"language\":\"en\",\"confidence\":0.9,\"needs_human\":false}"}}]}`)
	}))
	defer server.Close()
	client, err := llm.NewClient(llm.Config{BaseURL: server.URL, Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	h.SetAIDraftingClient(client)
	w := httptest.NewRecorder()
	h.handleAIDraft(w, draftMessageRequest(http.MethodPost, "owner", "1", `{}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("disabled status = %d, body %s", w.Code, w.Body.String())
	}
	if err := db.SetSetting(ctx, "owner", "ai_drafting_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAIAnalysis(ctx, storage.AIAnalysisResult{
		MessageID: 1, UserID: "owner", AccountID: "acc", AnalysisVersion: 1,
		RiskLabel: "phishing", RiskConfidence: 0.99, PromptVersion: "jev-v1",
	}, nil); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.handleAIDraft(w, draftMessageRequest(http.MethodPost, "owner", "1", `{}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("phishing status = %d, body %s", w.Code, w.Body.String())
	}
	h.SetAIDraftingClient(nil)
	w = httptest.NewRecorder()
	h.handleAIDraft(w, draftMessageRequest(http.MethodPost, "owner", "1", `{}`))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no client status = %d", w.Code)
	}
}

func TestAIDraftRejectsOversizedBodyAndInvalidID(t *testing.T) {
	h, _ := newAIHandler(t)
	client, err := llm.NewClient(llm.Config{BaseURL: "http://localhost:11434", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	h.SetAIDraftingClient(client)
	w := httptest.NewRecorder()
	h.handleAIDraft(w, draftMessageRequest(http.MethodPost, "owner", "abc", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.handleAIDraft(w, draftMessageRequest(http.MethodGet, "owner", "1", ``))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method = %d", w.Code)
	}
}
