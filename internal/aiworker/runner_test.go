package aiworker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aievents"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

type fakeClassifier struct {
	calls int
	err   error
}

func (f *fakeClassifier) Analyze(context.Context, storage.AnalysisJob) error {
	f.calls++
	return f.err
}

func newRunnerDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Write().Exec(`
		INSERT INTO users (id, username, username_normalized, name) VALUES ('default', 'default', 'default', 'Default');
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');
		INSERT INTO ai_analysis_jobs (id, account_id, message_id, internet_message_id, folder_id, state)
		VALUES ('job-1', 'acc', 1, '<one@example.com>', 'acc:INBOX', 'queued');`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRunnerCompletesClaimedJob(t *testing.T) {
	db := newRunnerDB(t)
	classifier := &fakeClassifier{}
	events := aievents.NewBus()
	updates, cancel := events.Subscribe("default")
	defer cancel()
	runner := &Runner{DB: db, Classifier: classifier, WorkerID: "worker-a", Lease: time.Minute, RetryDelay: func(int) time.Duration { return 0 }, Events: events}
	worked, err := runner.ProcessOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("ProcessOnce = %v, %v; want work", worked, err)
	}
	if classifier.calls != 1 {
		t.Fatalf("classifier calls = %d, want 1", classifier.calls)
	}
	var state string
	if err := db.Read().QueryRow(`SELECT state FROM ai_analysis_jobs WHERE id = 'job-1'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != storage.AnalysisStateDone {
		t.Fatalf("state = %q, want done", state)
	}
	select {
	case update := <-updates:
		if update.UserID != "default" || update.MessageID != 1 || update.State != storage.AnalysisStateDone {
			t.Fatalf("event = %+v", update)
		}
	default:
		t.Fatal("completion event not published")
	}
	worked, err = runner.ProcessOnce(context.Background())
	if err != nil || worked {
		t.Fatalf("second ProcessOnce = %v, %v; want idle", worked, err)
	}
}

func TestRunnerSkipsAnUnavailableMessageWithoutRetrying(t *testing.T) {
	db := newRunnerDB(t)
	classifier := &fakeClassifier{err: storage.ErrAnalysisSkipped}
	runner := &Runner{DB: db, Classifier: classifier, WorkerID: "worker-a", Lease: time.Minute, RetryDelay: func(int) time.Duration { return 0 }}
	worked, err := runner.ProcessOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("ProcessOnce = %v, %v; want skip", worked, err)
	}
	var state string
	if err := db.Read().QueryRow(`SELECT state FROM ai_analysis_jobs WHERE id = 'job-1'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != storage.AnalysisStateSkipped {
		t.Fatalf("state = %q, want skipped", state)
	}
}

func TestRunnerRetriesAFailedJob(t *testing.T) {
	db := newRunnerDB(t)
	classifier := &fakeClassifier{err: errors.New("temporary upstream error")}
	runner := &Runner{DB: db, Classifier: classifier, WorkerID: "worker-a", Lease: time.Minute, RetryDelay: func(int) time.Duration { return -time.Second }}
	worked, err := runner.ProcessOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("ProcessOnce = %v, %v; want work", worked, err)
	}
	var state, lastError string
	var attempts int
	if err := db.Read().QueryRow(`SELECT state, attempts, last_error FROM ai_analysis_jobs WHERE id = 'job-1'`).Scan(&state, &attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if state != storage.AnalysisStateQueued || attempts != 1 || lastError == "" {
		t.Fatalf("job = state:%q attempts:%d error:%q, want queued retry", state, attempts, lastError)
	}
}
