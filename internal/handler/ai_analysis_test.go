package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func aiMessageRequest(method, user, id string) *http.Request {
	r := aiRequest(method, "/api/ai/messages/"+id+"/analysis", user, "")
	r.SetPathValue("id", id)
	return r
}

func TestAIAnalysisAPIReadsOnlyTheOwnersResult(t *testing.T) {
	h, db := newAIHandler(t)
	ctx := context.Background()
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'owner', 'owner@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email) VALUES (1, 'acc', '<one@example.com>', 'One', 'alice@example.com');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAILegitimacyProfile(ctx, storage.AILegitimacyProfile{
		AccountID: "acc", SenderEmail: "alice@example.com", SenderDomain: "example.com",
		Score: 0.82, State: "trusted", Allowlisted: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAIAnalysis(ctx, storage.AIAnalysisResult{
		MessageID: 1, UserID: "owner", AccountID: "acc", AnalysisVersion: 1,
		Model: "jev-test", PriorityLabel: "high", PriorityConfidence: 0.9,
		RiskLabel: "legit", RiskConfidence: 0.95, IntentLabel: "reply_required",
		IntentConfidence: 0.8, ProjectLabel: "employment", ProjectConfidence: 0.91,
		PromptVersion: "jev-v1",
	}, []storage.AIAnalysisAnswer{
		{MessageID: 1, QuestionKey: "priority", Type: "score", Score: 2, Confidence: 0.9, ProbabilitiesJSON: `[0.1,0.2,0.7]`},
	}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handleAIMessageAnalysis(w, aiMessageRequest(http.MethodGet, "owner", "1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var payload aiAnalysisPayload
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.PriorityLabel != "high" || payload.ProjectLabel != "employment" || len(payload.Answers) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	w = httptest.NewRecorder()
	h.handleAIMessageAnalysis(w, aiMessageRequest(http.MethodGet, "other", "1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-user status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.handleAIMessageAnalysis(w, aiMessageRequest(http.MethodGet, "owner", "999"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", w.Code)
	}
	actionID, err := db.RecordAIAutomationAction(ctx, storage.AIAutomationAction{
		MessageID: 1, Kind: "move_junk", GateJSON: `{"source":"acc:INBOX","destination":"acc:Junk"}`,
		ExecutedAt: time.Now().UTC(), UndoDeadline: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.handleAIAnalysisPanel(w, aiMessageRequest(http.MethodGet, "owner", "1"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "High") || !strings.Contains(w.Body.String(), "Reanalyze") ||
		!strings.Contains(w.Body.String(), "alice@example.com") || !strings.Contains(w.Body.String(), "Trusted") ||
		!strings.Contains(w.Body.String(), "Moved to Junk") || !strings.Contains(w.Body.String(), actionID) {
		t.Fatalf("panel status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.handleAIAnalysisPanel(w, aiMessageRequest(http.MethodGet, "owner", "999"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("missing panel status = %d", w.Code)
	}
}

func TestAIReanalyzeAPIQueuesOneJobAndHonorsFlags(t *testing.T) {
	h, db := newAIHandler(t)
	ctx := context.Background()
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'owner', 'owner@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:Sent', 'acc', 'Sent', 'Sent', 'sent');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handleAIReanalyze(w, aiMessageRequest(http.MethodPost, "owner", "1"))
	if w.Code != http.StatusConflict {
		t.Fatalf("disabled status = %d, body %s", w.Code, w.Body.String())
	}
	if err := db.SetSetting(ctx, "owner", "ai_analysis_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.handleAIReanalyze(w, aiMessageRequest(http.MethodPost, "owner", "1"))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var first map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil || first["job_id"] == "" {
		t.Fatalf("response = %v, %v", first, err)
	}
	w = httptest.NewRecorder()
	h.handleAIReanalyze(w, aiMessageRequest(http.MethodPost, "owner", "1"))
	var second map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil || second["job_id"] != first["job_id"] {
		t.Fatalf("second response = %v, %v", second, err)
	}
	var jobs int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_analysis_jobs`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("jobs = %d, %v", jobs, err)
	}
	hx := aiMessageRequest(http.MethodPost, "owner", "1")
	hx.Header.Set("HX-Request", "true")
	w = httptest.NewRecorder()
	h.handleAIReanalyze(w, hx)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), "analysis queued") {
		t.Fatalf("htmx status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.handleAIReanalyze(w, aiMessageRequest(http.MethodPost, "other", "1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-user status = %d", w.Code)
	}
	if _, err := db.Write().Exec(`UPDATE message_folder_state SET folder_id = 'acc:Sent' WHERE message_id = 1`); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.handleAIReanalyze(w, aiMessageRequest(http.MethodPost, "owner", "1"))
	if w.Code != http.StatusConflict {
		t.Fatalf("sent folder status = %d", w.Code)
	}
	h.SetAIWorkerReady(false)
	w = httptest.NewRecorder()
	h.handleAIReanalyze(w, aiMessageRequest(http.MethodPost, "owner", "1"))
	if w.Code != http.StatusConflict {
		t.Fatalf("no worker status = %d", w.Code)
	}
}
