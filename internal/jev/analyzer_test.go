package jev

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func TestAnalyzerPersistsAValidJevResult(t *testing.T) {
	db, err := storage.New(filepath.Join(t.TempDir(), "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rawPath := filepath.Join(t.TempDir(), "message.eml")
	raw := "From: Alice <alice@example.com>\r\nTo: Me <me@example.com>\r\nSubject: Interview\r\nContent-Type: text/plain\r\n\r\nBonjour\r\n"
	if err := os.WriteFile(rawPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Write().Exec(`
		INSERT INTO users (id, username, username_normalized, name) VALUES ('default', 'default', 'default', 'Default');
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'me@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, raw_path) VALUES (1, 'acc', '<one@example.com>', 'Interview', ?);
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`, rawPath); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, validResponse)
	}))
	defer server.Close()
	client, err := NewClient(Config{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	analyzer := &Analyzer{DB: db, Client: client}
	err = analyzer.Analyze(context.Background(), storage.AnalysisJob{
		ID: "job-1", UserID: "default", AccountID: "acc", MessageID: 1, AnalysisVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, answers, err := db.GetAIAnalysisForUser(context.Background(), 1, "default")
	if err != nil {
		t.Fatal(err)
	}
	if result.PriorityLabel != "high" || result.RiskLabel != "legit" || result.ProjectLabel != "employment" {
		t.Fatalf("result = %+v", result)
	}
	if result.IntentLabel != "reply_required" || result.PromptVersion != DefaultPromptVersion {
		t.Fatalf("result = %+v", result)
	}
	if len(answers) != 4 {
		t.Fatalf("answers = %+v", answers)
	}
	if _, err := db.Write().Exec(`DELETE FROM message_folder_state WHERE message_id = 1`); err != nil {
		t.Fatal(err)
	}
	err = analyzer.Analyze(context.Background(), storage.AnalysisJob{
		ID: "job-2", UserID: "default", AccountID: "acc", MessageID: 1, AnalysisVersion: 1,
	})
	if !errors.Is(err, storage.ErrAnalysisSkipped) {
		t.Fatalf("missing message error = %v, want skip", err)
	}
}
