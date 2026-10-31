package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestAIAnalysisResultsAreOwnerScopedAndReplaceAnswers(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');
		INSERT INTO users (id, username, username_normalized, name) VALUES ('other', 'other', 'other', 'Other');`); err != nil {
		t.Fatal(err)
	}
	result := AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		Model: "jev-test", PriorityLabel: "high", PriorityConfidence: 0.9,
		RiskLabel: "legit", RiskConfidence: 0.95, IntentLabel: "reply_required",
		IntentConfidence: 0.8, ProjectLabel: "employment", ProjectConfidence: 0.91,
		InputTokens: 100, OutputTokens: 0, PromptVersion: "jev-v1",
	}
	answers := []AIAnalysisAnswer{
		{MessageID: 1, QuestionKey: "priority", Type: "score", Score: 2, Confidence: 0.9, ProbabilitiesJSON: `[0.1,0.2,0.7]`},
		{MessageID: 1, QuestionKey: "risk", Type: "choice", Choice: "legit", Confidence: 0.95, ProbabilitiesJSON: `{"legit":0.95}`},
	}
	if err := db.SaveAIAnalysis(ctx, result, answers); err != nil {
		t.Fatal(err)
	}
	got, gotAnswers, err := db.GetAIAnalysisForUser(ctx, 1, "default")
	if err != nil {
		t.Fatal(err)
	}
	if got.PriorityLabel != "high" || got.ProjectLabel != "employment" || got.UserID != "default" || len(gotAnswers) != 2 {
		t.Fatalf("result = %+v answers = %+v", got, gotAnswers)
	}
	if crossResult, crossAnswers, err := db.GetAIAnalysisForUser(ctx, 1, "other"); !errors.Is(err, sql.ErrNoRows) || crossResult != nil || crossAnswers != nil {
		t.Fatalf("cross-user read = %+v, %+v, %v", crossResult, crossAnswers, err)
	}
	if err := db.SaveAIAnalysis(ctx, result, answers[:1]); err != nil {
		t.Fatal(err)
	}
	_, gotAnswers, err = db.GetAIAnalysisForUser(ctx, 1, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotAnswers) != 1 || gotAnswers[0].QuestionKey != "priority" {
		t.Fatalf("answers after replacement = %+v", gotAnswers)
	}
	result.UserID = "other"
	if err := db.SaveAIAnalysis(ctx, result, answers[:1]); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user write = %v, want no rows", err)
	}
}
