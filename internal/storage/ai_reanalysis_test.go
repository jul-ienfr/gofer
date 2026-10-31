package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestEnqueueAnalysisForUserRequeuesFinishedJobs(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:Sent', 'acc', 'Sent', 'Sent', 'sent');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '', 'No ID');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);
		INSERT INTO users (id, username, username_normalized, name) VALUES ('other', 'other', 'other', 'Other');`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnqueueAnalysisForUser(ctx, "default", 1); !errors.Is(err, ErrAnalysisDisabled) {
		t.Fatalf("disabled enqueue = %v", err)
	}
	if err := db.SetSetting(ctx, "default", "ai_analysis_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	jobID, err := db.EnqueueAnalysisForUser(ctx, "default", 1)
	if err != nil || jobID == "" {
		t.Fatalf("enqueue = %q, %v", jobID, err)
	}
	sameID, err := db.EnqueueAnalysisForUser(ctx, "default", 1)
	if err != nil || sameID != jobID {
		t.Fatalf("second enqueue = %q, %v; want %q", sameID, err, jobID)
	}
	job, err := db.ClaimAnalysisJob(ctx, "worker", time.Minute)
	if err != nil || job == nil || job.ID != jobID {
		t.Fatalf("claim = %+v, %v", job, err)
	}
	if err := db.CompleteAnalysisJob(ctx, jobID, "worker"); err != nil {
		t.Fatal(err)
	}
	requeued, err := db.EnqueueAnalysisForUser(ctx, "default", 1)
	if err != nil || requeued != jobID {
		t.Fatalf("requeue = %q, %v", requeued, err)
	}
	var state string
	var attempts int
	if err := db.Read().QueryRow(`SELECT state, attempts FROM ai_analysis_jobs WHERE id = ?`, jobID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != AnalysisStateQueued || attempts != 0 {
		t.Fatalf("state = %q attempts = %d", state, attempts)
	}
	if _, err := db.Write().Exec(`UPDATE message_folder_state SET folder_id = 'acc:Sent' WHERE message_id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnqueueAnalysisForUser(ctx, "default", 1); !errors.Is(err, ErrAnalysisFolderNotAllowed) {
		t.Fatalf("sent folder enqueue = %v", err)
	}
	if _, err := db.EnqueueAnalysisForUser(ctx, "other", 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user enqueue = %v", err)
	}
}
