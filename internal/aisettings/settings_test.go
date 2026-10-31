package aisettings

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func newSettingsDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Write().Exec(`INSERT INTO users (id, username, username_normalized, name) VALUES ('owner', 'owner', 'owner', 'Owner')`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSettingsDefaultToOffAndPersist(t *testing.T) {
	db := newSettingsDB(t)
	ctx := context.Background()
	got, err := Load(ctx, db, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Settings{}) {
		t.Fatalf("defaults = %+v, want all disabled", got)
	}
	want := Settings{AnalysisEnabled: true, DraftingEnabled: true}
	if err := Save(ctx, db, "owner", want); err != nil {
		t.Fatal(err)
	}
	got, err = Load(ctx, db, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loaded = %+v, want %+v", got, want)
	}
	if err := Save(ctx, db, "owner", Settings{}); err != nil {
		t.Fatal(err)
	}
	got, err = Load(ctx, db, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Settings{}) {
		t.Fatalf("disabled settings = %+v, want all disabled", got)
	}
}

func TestSettingsAreScopedToTheUser(t *testing.T) {
	db := newSettingsDB(t)
	ctx := context.Background()
	if _, err := db.Write().Exec(`INSERT INTO users (id, username, username_normalized, name) VALUES ('other', 'other', 'other', 'Other')`); err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, db, "owner", Settings{AnalysisEnabled: true}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ctx, db, "other")
	if err != nil {
		t.Fatal(err)
	}
	if got.AnalysisEnabled {
		t.Fatal("settings leaked to another user")
	}
}
