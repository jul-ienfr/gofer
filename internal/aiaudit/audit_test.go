package aiaudit

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func newAuditDB(t *testing.T) *storage.DB {
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
	return db
}

func TestAuditRecordIsScopedAndOrdered(t *testing.T) {
	db := newAuditDB(t)
	ctx := context.Background()
	if _, err := Record(ctx, db, Event{UserID: "owner", ActorType: ActorUser, ActorID: "owner", Action: "settings.update", AfterJSON: `{"analysis_enabled":true}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(ctx, db, Event{UserID: "owner", Action: "analysis.complete", SubjectType: "message", SubjectID: "42"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(ctx, db, Event{UserID: "other", Action: "settings.update"}); err != nil {
		t.Fatal(err)
	}
	events, err := List(ctx, db, "owner", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("owner events = %d, want 2", len(events))
	}
	if events[0].Action != "analysis.complete" || events[1].Action != "settings.update" {
		t.Fatalf("events = %+v, want newest first", events)
	}
	if events[1].BeforeJSON != "{}" || events[1].AfterJSON != `{"analysis_enabled":true}` {
		t.Fatalf("audit payloads = %q %q", events[1].BeforeJSON, events[1].AfterJSON)
	}
	other, err := List(ctx, db, "other", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].UserID != "other" {
		t.Fatalf("other events = %+v", other)
	}
}

func TestAuditRejectsInvalidEvents(t *testing.T) {
	db := newAuditDB(t)
	ctx := context.Background()
	if _, err := Record(ctx, db, Event{UserID: "owner", Action: ""}); err == nil {
		t.Fatal("empty action accepted")
	}
	if _, err := Record(ctx, db, Event{UserID: "owner", Action: "settings.update", ActorType: "robot"}); err == nil {
		t.Fatal("invalid actor accepted")
	}
	if _, err := Record(ctx, db, Event{UserID: "owner", Action: "settings.update", BeforeJSON: "not-json"}); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
