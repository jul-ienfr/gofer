package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRestoreRemovesStaleFiles(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "gofer.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (7)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "attachments"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "attachments", "kept.txt"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "backup.tar.gz")
	if _, err := Create(ctx, CreateOptions{DataDir: dataDir, DatabasePath: dbPath, OutputPath: archive}); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "restored")
	for _, stale := range []string{
		"stale.txt",
		filepath.Join("attachments", "stale.txt"),
		filepath.Join("cache", "stale.bin"),
		filepath.Join("backups", "old.tar.gz"),
	} {
		path := filepath.Join(target, stale)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Restore(ctx, archive, RestoreOptions{DataDir: target, Force: true}); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"stale.txt", filepath.Join("attachments", "stale.txt"), filepath.Join("cache", "stale.bin")} {
		if _, err := os.Stat(filepath.Join(target, gone)); !os.IsNotExist(err) {
			t.Errorf("%s still exists after restore", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "cache")); !os.IsNotExist(err) {
		t.Errorf("empty stale directory still exists after restore")
	}
	if _, err := os.Stat(filepath.Join(target, "attachments", "kept.txt")); err != nil {
		t.Errorf("restored attachment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "backups", "old.tar.gz")); err != nil {
		t.Errorf("existing backup was removed: %v", err)
	}
}
