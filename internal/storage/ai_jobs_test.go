package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestFreshDatabaseHasAISchema(t *testing.T) {
	db := newContactsTestDB(t)
	var version int
	if err := db.Read().QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion || CurrentSchemaVersion < 141 {
		t.Fatalf("schema version = %d, want %d (at least 141)", version, CurrentSchemaVersion)
	}
	for _, table := range []string{
		"ai_analysis_jobs", "ai_analysis_results", "ai_analysis_answers",
		"ai_legitimacy_profiles", "ai_projects", "ai_project_routes",
		"ai_message_projects", "ai_connectors", "ai_routing_outbox",
		"ai_llm_providers", "ai_llm_generations", "ai_automation_actions",
		"ai_unsubscribe_attempts", "ai_api_tokens", "ai_audit_events", "ai_prompt_versions",
	} {
		var count int
		if err := db.Read().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %s missing", table)
		}
	}
}

func TestAIMigrationsUpgradeFromV132(t *testing.T) {
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email)
			VALUES (1, 'acc', '<kept@example.com>', 'Keep me', 'alice@example.com');`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"ai_analysis_answers", "ai_analysis_results", "ai_analysis_jobs",
		"ai_legitimacy_profiles", "ai_message_projects", "ai_project_routes",
		"ai_projects", "ai_routing_outbox", "ai_connectors",
		"ai_llm_generations", "ai_llm_providers", "ai_automation_actions",
		"ai_unsubscribe_attempts", "ai_api_tokens", "ai_audit_events", "ai_prompt_versions",
	} {
		if _, err := db.Write().Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Write().Exec(`DELETE FROM schema_version`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Write().Exec(`INSERT INTO schema_version (version) VALUES (132)`); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(t.TempDir(), "pre-ai-backup.db")
	if _, err := db.Write().Exec(`VACUUM INTO ?`, backupPath); err != nil {
		t.Fatalf("backup before migration: %v", err)
	}
	if err := db.migrate(); err != nil {
		t.Fatalf("migrate from 132: %v", err)
	}
	var version int
	if err := db.Read().QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version after upgrade = %d, want %d", version, CurrentSchemaVersion)
	}
	var subject string
	if err := db.Read().QueryRow(`SELECT subject FROM messages WHERE id = 1`).Scan(&subject); err != nil || subject != "Keep me" {
		t.Fatalf("existing message changed: %q, %v", subject, err)
	}
	var projectColumns int
	if err := db.Read().QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('ai_projects')
		WHERE name IN ('allow_external_routing', 'allow_llm_draft', 'allow_auto_spam', 'data_classification')`).Scan(&projectColumns); err != nil || projectColumns != 4 {
		t.Fatalf("project permission columns = %d, %v", projectColumns, err)
	}
	for _, table := range []string{"ai_analysis_jobs", "ai_prompt_versions"} {
		var count int
		if err := db.Read().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %s missing after upgrade", table)
		}
	}
	restored, err := New(backupPath)
	if err != nil {
		t.Fatalf("restore pre-migration backup: %v", err)
	}
	defer restored.Close()
	if err := restored.Read().QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("restored schema version = %d, %v", version, err)
	}
	if err := restored.Read().QueryRow(`SELECT subject FROM messages WHERE id = 1`).Scan(&subject); err != nil || subject != "Keep me" {
		t.Fatalf("restored message changed: %q, %v", subject, err)
	}
}

func TestAnalysisEnqueueIsIdempotentAndSkipsDraftFolders(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:Sent', 'acc', 'Sent', 'Sent', 'sent');`); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "default", "ai_analysis_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	inbox := SyncMessage{AccountID: "acc", FolderID: "acc:INBOX", RemoteUID: 1, MessageID: "<one@example.com>", Subject: "One", Arrived: true}
	if err := db.UpsertSyncMessages(ctx, []SyncMessage{inbox}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSyncMessages(ctx, []SyncMessage{inbox}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_analysis_jobs WHERE state = 'queued'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("queued jobs = %d, want 1 after a repeated arrival", count)
	}
	sent := SyncMessage{AccountID: "acc", FolderID: "acc:Sent", RemoteUID: 2, MessageID: "<two@example.com>", Subject: "Two", Arrived: true}
	if err := db.UpsertSyncMessages(ctx, []SyncMessage{sent}); err != nil {
		t.Fatal(err)
	}
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_analysis_jobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("jobs after sent arrival = %d, want 1", count)
	}
	noID := []SyncMessage{
		{AccountID: "acc", FolderID: "acc:INBOX", RemoteUID: 3, Subject: "No ID one", Arrived: true},
		{AccountID: "acc", FolderID: "acc:INBOX", RemoteUID: 4, Subject: "No ID two", Arrived: true},
	}
	if err := db.UpsertSyncMessages(ctx, noID); err != nil {
		t.Fatal(err)
	}
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_analysis_jobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("jobs after messages without Message-ID = %d, want 3", count)
	}
}

func TestAnalysisEnqueuesOneJobPerArrivedMessageAtVolume(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');`); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "default", "ai_analysis_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	const total = 1000
	messages := make([]SyncMessage, 0, total)
	for i := 1; i <= total; i++ {
		messages = append(messages, SyncMessage{
			AccountID: "acc",
			FolderID:  "acc:INBOX",
			RemoteUID: uint32(i),
			MessageID: fmt.Sprintf("<bulk-%d@example.com>", i),
			Subject:   fmt.Sprintf("Bulk %d", i),
			Arrived:   true,
		})
	}
	if err := db.UpsertSyncMessages(ctx, messages); err != nil {
		t.Fatal(err)
	}
	var jobs, distinct int
	if err := db.Read().QueryRow(`SELECT COUNT(*), COUNT(DISTINCT message_id) FROM ai_analysis_jobs`).Scan(&jobs, &distinct); err != nil {
		t.Fatal(err)
	}
	if jobs != total || distinct != total {
		t.Fatalf("jobs = %d distinct messages = %d, want %d", jobs, distinct, total)
	}
	claimed := 0
	for {
		job, err := db.ClaimAnalysisJob(ctx, "bulk-worker", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if job == nil {
			break
		}
		claimed++
		if err := db.CompleteAnalysisJob(ctx, job.ID, "bulk-worker"); err != nil {
			t.Fatal(err)
		}
	}
	if claimed != total {
		t.Fatalf("claimed jobs = %d, want %d", claimed, total)
	}
}

func TestAnalysisJobsAreNotQueuedWhenDisabled(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');`); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSyncMessages(ctx, []SyncMessage{{
		AccountID: "acc", FolderID: "acc:INBOX", RemoteUID: 1, MessageID: "<one@example.com>", Arrived: true,
	}}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_analysis_jobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("jobs while disabled = %d, want 0", count)
	}
}

func TestAnalysisJobClaimLeaseRetryAndRecovery(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');`); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "default", "ai_analysis_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Write().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := enqueueAnalysisTx(ctx, tx, 1, "acc", "acc:INBOX", "<one@example.com>", CurrentAnalysisVersion); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	job, err := db.ClaimAnalysisJob(ctx, "worker-a", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim = %#v, %v", job, err)
	}
	if job.Attempts != 1 || job.MessageID != 1 || job.UserID != "default" {
		t.Fatalf("claimed job = %#v", job)
	}
	if second, err := db.ClaimAnalysisJob(ctx, "worker-b", time.Minute); err != nil || second != nil {
		t.Fatalf("second claim = %#v, %v; want no job", second, err)
	}
	if err := db.CompleteAnalysisJob(ctx, job.ID, "wrong-worker"); !errors.Is(err, ErrAnalysisJobLeaseLost) {
		t.Fatalf("wrong worker completion = %v, want lease lost", err)
	}
	if err := db.CompleteAnalysisJob(ctx, job.ID, "worker-a"); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.Read().QueryRow(`SELECT state FROM ai_analysis_jobs WHERE id = ?`, job.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != AnalysisStateDone {
		t.Fatalf("state = %q, want done", state)
	}
}

func TestAnalysisJobRecoveryAfterExpiredLease(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');`); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "default", "ai_analysis_enabled", "1"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Write().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := enqueueAnalysisTx(ctx, tx, 1, "acc", "acc:INBOX", "<one@example.com>", CurrentAnalysisVersion); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimAnalysisJob(ctx, "worker-a", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim = %#v, %v", job, err)
	}
	if _, err := db.Write().Exec(`UPDATE ai_analysis_jobs SET lease_expires_at = datetime('now', '-1 minute') WHERE id = ?`, job.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := db.RecoverExpiredAnalysisJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	job, err = db.ClaimAnalysisJob(ctx, "worker-b", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim after recovery = %#v, %v", job, err)
	}
	if job.Attempts != 3 {
		t.Fatalf("attempts after recovery = %d, want 3", job.Attempts)
	}
	if err := db.RetryAnalysisJob(ctx, job.ID, "worker-b", "network timeout", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.Read().QueryRow(`SELECT state FROM ai_analysis_jobs WHERE id = ?`, job.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != AnalysisStateQueued {
		t.Fatalf("state after retry = %q, want queued", state)
	}
}

func TestAnalysisMigrationLeavesExistingVersionIntact(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := applyAIMigrations(db.Write(), CurrentSchemaVersion); err != nil {
		t.Fatalf("apply at %d: %v", CurrentSchemaVersion, err)
	}
	var version int
	if err := db.Read().QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("version = %d, want %d", version, CurrentSchemaVersion)
	}
}
