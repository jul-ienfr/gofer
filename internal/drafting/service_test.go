package drafting

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/llm"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

type fakeGenerator struct {
	input llm.DraftInput
	draft llm.Draft
	err   error
	calls int
}

func (f *fakeGenerator) Draft(_ context.Context, input llm.DraftInput) (llm.Draft, llm.Completion, error) {
	f.calls++
	f.input = input
	return f.draft, llm.Completion{Model: "fake", Usage: llm.Usage{PromptTokens: 42, CompletionTokens: 12}}, f.err
}

func draftFixture(t *testing.T) (*storage.DB, *fakeGenerator) {
	t.Helper()
	db := newDraftingTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, display_name, provider) VALUES ('acc', 'default', 'ana@example.com', 'Ana', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email, preview_text, thread_id)
			VALUES (1, 'acc', '<one@example.com>', 'Interview', 'alice@example.com', 'Can we meet Tuesday?', 'thread-1');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`); err != nil {
		t.Fatal(err)
	}
	result := storage.AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		Model: "jev", RiskLabel: "legit", RiskConfidence: 0.92,
		PriorityLabel: "high", IntentLabel: "reply_required", ProjectLabel: "employment",
		PromptVersion: "jev-v1",
	}
	if err := db.SaveAIAnalysis(context.Background(), result, nil); err != nil {
		t.Fatal(err)
	}
	return db, &fakeGenerator{draft: llm.Draft{ReplyText: "Bonjour, mardi me convient.", Language: "fr", Confidence: 0.9}}
}

func newDraftingTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.New(t.TempDir() + "/gofer.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Write().Exec(`INSERT OR IGNORE INTO users (id, username, username_normalized, name) VALUES ('default', 'default', 'default', 'Default')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestGenerateHonorsProjectDraftPermission(t *testing.T) {
	db, generator := draftFixture(t)
	deny := false
	if _, err := db.CreateAIProject(context.Background(), storage.AIProject{
		UserID: "default", Name: "Employment", Slug: "employment", AllowLLMDraft: &deny,
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{DB: db, Generator: generator}
	if _, err := service.Generate(context.Background(), Request{UserID: "default", MessageID: 1}); !errors.Is(err, ErrProjectDraftDenied) {
		t.Fatalf("error = %v", err)
	}
	if generator.calls != 0 {
		t.Fatal("denied project reached the model")
	}
}

func TestGenerateDraftsOwnedMessageAndRecordsUsage(t *testing.T) {
	db, generator := draftFixture(t)
	service := &Service{DB: db, Generator: generator}
	result, err := service.Generate(context.Background(), Request{
		UserID: "default", MessageID: 1, Instruction: "Réponds brièvement", Tone: "professionnel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Draft.Language != "fr" || result.Model != "fake" || result.Usage.PromptTokens != 42 {
		t.Fatalf("result = %+v", result)
	}
	if result.Compose.AccountID != "acc" || result.Compose.To != "alice@example.com" || result.Compose.Subject != "Re: Interview" {
		t.Fatalf("compose = %+v", result.Compose)
	}
	if generator.calls != 1 || generator.input.Identity.Email != "ana@example.com" ||
		generator.input.Tone != "professionnel" || len(generator.input.Thread) == 0 {
		t.Fatalf("input = %+v", generator.input)
	}
	var status string
	var inputTokens int
	if err := db.Read().QueryRow(`SELECT status, input_tokens FROM ai_llm_generations WHERE message_id = 1`).Scan(&status, &inputTokens); err != nil {
		t.Fatal(err)
	}
	if status != "done" || inputTokens != 42 {
		t.Fatalf("generation status = %q tokens = %d", status, inputTokens)
	}
}

func TestGenerateRejectsUnsafeOrUnanalyzedMessages(t *testing.T) {
	db, generator := draftFixture(t)
	if _, err := db.Write().Exec(`UPDATE ai_analysis_results SET risk_label = 'phishing', risk_confidence = 0.99 WHERE message_id = 1`); err != nil {
		t.Fatal(err)
	}
	service := &Service{DB: db, Generator: generator}
	if _, err := service.Generate(context.Background(), Request{UserID: "default", MessageID: 1}); !errors.Is(err, ErrUnsafeAnalysis) {
		t.Fatalf("phishing error = %v", err)
	}
	if generator.calls != 0 {
		t.Fatal("unsafe message reached the model")
	}
	if _, err := db.Write().Exec(`DELETE FROM ai_analysis_results WHERE message_id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Generate(context.Background(), Request{UserID: "default", MessageID: 1}); !errors.Is(err, ErrAnalysisMissing) {
		t.Fatalf("missing analysis error = %v", err)
	}
	if _, err := service.Generate(context.Background(), Request{UserID: "other", MessageID: 1}); !errors.Is(err, ErrMessageNotFound) && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user error = %v", err)
	}
}
