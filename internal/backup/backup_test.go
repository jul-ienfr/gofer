package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCreateVerifyRestore(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "gofer.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE schema_version (version INTEGER PRIMARY KEY)`,
		`INSERT INTO schema_version (version) VALUES (7)`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`,
		`INSERT INTO notes (body) VALUES ('kept across backup')`,
		`PRAGMA journal_mode=WAL`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "attachments"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "attachments", "hello.txt"), []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "backup.tar.gz")
	manifest, err := Create(ctx, CreateOptions{DataDir: dataDir, DatabasePath: dbPath, OutputPath: archive})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 7 {
		t.Fatalf("schema version = %d, want 7", manifest.SchemaVersion)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(manifest.Files))
	}
	if _, err := Create(ctx, CreateOptions{DataDir: dataDir, DatabasePath: dbPath, OutputPath: archive}); err == nil {
		t.Fatal("expected existing archive to be rejected")
	}
	verified, err := Verify(archive)
	if err != nil {
		t.Fatal(err)
	}
	if verified.DatabasePath != "gofer.db" {
		t.Fatalf("database path = %q", verified.DatabasePath)
	}

	restoredDir := filepath.Join(t.TempDir(), "restored")
	restored, err := Restore(ctx, archive, RestoreOptions{DataDir: restoredDir})
	if err != nil {
		t.Fatal(err)
	}
	if restored.SchemaVersion != 7 {
		t.Fatalf("restored schema version = %d", restored.SchemaVersion)
	}
	restoredDB, err := sql.Open("sqlite", filepath.Join(restoredDir, "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	var body string
	if err := restoredDB.QueryRow(`SELECT body FROM notes`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if err := restoredDB.Close(); err != nil {
		t.Fatal(err)
	}
	if body != "kept across backup" {
		t.Fatalf("body = %q", body)
	}
	attachment, err := os.ReadFile(filepath.Join(restoredDir, "attachments", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(attachment) != "attachment" {
		t.Fatalf("attachment = %q", attachment)
	}

	if _, err := Restore(ctx, archive, RestoreOptions{DataDir: restoredDir}); err == nil {
		t.Fatal("expected non-empty restore without force to fail")
	}
	if _, err := Restore(ctx, archive, RestoreOptions{DataDir: restoredDir, Force: true}); err != nil {
		t.Fatal(err)
	}
	safety, err := filepath.Glob(filepath.Join(restoredDir, "backups", "pre-restore-*.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(safety) != 1 {
		t.Fatalf("safety backups = %d, want 1", len(safety))
	}
}

func TestRestoreRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "evil.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	body := []byte("escape")
	if err := tw.WriteHeader(&tar.Header{Name: "../evil.txt", Mode: 0o600, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "data")
	if _, err := Restore(context.Background(), archive, RestoreOptions{DataDir: dest}); err == nil {
		t.Fatal("expected traversal archive to be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("traversal file exists: %v", err)
	}
}
