package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func TestAIAutomationUndoRestoresTheOriginalFolder(t *testing.T) {
	h, db := newAIHandler(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'owner', 'owner@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES
			('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox'),
			('acc:Junk', 'acc', 'Junk', 'Junk', 'junk');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email) VALUES
			(1, 'acc', '<spam@example.com>', 'Spam', 'spam@example.com');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:Junk', 0);`); err != nil {
		t.Fatal(err)
	}
	actionID, err := db.RecordAIAutomationAction(context.Background(), storage.AIAutomationAction{
		MessageID: 1, Kind: "move_junk", GateJSON: `{"source":"acc:INBOX","destination":"acc:Junk"}`,
		ExecutedAt: time.Now().UTC(), UndoDeadline: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := aiRequest(http.MethodPost, "/api/ai/automation/"+actionID+"/undo", "owner", "")
	request.SetPathValue("id", actionID)
	w := httptest.NewRecorder()
	h.handleAIAutomationUndo(w, request)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var folderID, state string
	if err := db.Read().QueryRow(`SELECT folder_id FROM message_folder_state WHERE message_id = 1`).Scan(&folderID); err != nil || folderID != "acc:INBOX" {
		t.Fatalf("folder = %q, %v", folderID, err)
	}
	if err := db.Read().QueryRow(`SELECT state FROM ai_automation_actions WHERE id = ?`, actionID).Scan(&state); err != nil || state != "undone" {
		t.Fatalf("state = %q, %v", state, err)
	}
	request = aiRequest(http.MethodPost, "/api/ai/automation/"+actionID+"/undo", "owner", "")
	request.SetPathValue("id", actionID)
	w = httptest.NewRecorder()
	h.handleAIAutomationUndo(w, request)
	if w.Code != http.StatusConflict {
		t.Fatalf("second undo status = %d, body %s", w.Code, w.Body.String())
	}
}

func TestAIAutomationUndoRejectsAnotherOwner(t *testing.T) {
	h, db := newAIHandler(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'owner', 'owner@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email) VALUES
			(1, 'acc', '<spam@example.com>', 'Spam', 'spam@example.com');`); err != nil {
		t.Fatal(err)
	}
	actionID, err := db.RecordAIAutomationAction(context.Background(), storage.AIAutomationAction{
		MessageID: 1, Kind: "move_junk", GateJSON: `{"source":"acc:INBOX","destination":"acc:Junk"}`,
		ExecutedAt: time.Now().UTC(), UndoDeadline: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := aiRequest(http.MethodPost, "/api/ai/automation/"+actionID+"/undo", "other", "")
	request.SetPathValue("id", actionID)
	w := httptest.NewRecorder()
	h.handleAIAutomationUndo(w, request)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
}
