package server

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/backup"
	"github.com/cristianadrielbraun/gofer/internal/storage"

	_ "modernc.org/sqlite"
)

func createBackupCommandFixture(t *testing.T) (string, string) {
	t.Helper()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "gofer.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE schema_version (version INTEGER PRIMARY KEY)`,
		`INSERT INTO schema_version (version) VALUES (7)`,
		`CREATE TABLE notes (body TEXT)`,
		`INSERT INTO notes (body) VALUES ('backup command')`,
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
	if err := os.WriteFile(filepath.Join(dataDir, "attachments", "note.txt"), []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir, dbPath
}

func TestStorageBackupAndRestoreCommands(t *testing.T) {
	dataDir, dbPath := createBackupCommandFixture(t)
	archive := filepath.Join(t.TempDir(), "backup.tar.gz")

	var stdout, stderr bytes.Buffer
	handled, code := RunStorageCommand(context.Background(), []string{
		"storage", "backup", "--dir", dataDir, "--db", dbPath, "--out", archive,
	}, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("backup handled %t, exit %d, stderr:\n%s", handled, code, stderr.String())
	}
	for _, want := range []string{"backup created: " + archive, "schema version: 7", "files: 2"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("backup output missing %q:\n%s", want, stdout.String())
		}
	}
	manifest, err := backup.Verify(archive)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 7 || manifest.DatabasePath != "gofer.db" {
		t.Fatalf("manifest = %#v", manifest)
	}

	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "verify", "--from", archive,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify exit %d, stderr:\n%s", code, stderr.String())
	}
	for _, want := range []string{"backup verified: " + archive, "schema version: 7", "files: 2"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("verify output missing %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "backup", "--dir", dataDir, "--db", dbPath, "--out", archive,
	}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("existing archive: exit %d, stderr:\n%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "backup", "--dir", dataDir, "--db", dbPath, "--out", archive, "--force",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("force backup exit %d, stderr:\n%s", code, stderr.String())
	}

	restoredDir := filepath.Join(t.TempDir(), "restored")
	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "restore", "--from", archive, "--dir", restoredDir,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("restore exit %d, stderr:\n%s", code, stderr.String())
	}
	for _, want := range []string{"backup restored: " + archive, "data directory: " + restoredDir, "schema version: 7", "files: 2"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("restore output missing %q:\n%s", want, stdout.String())
		}
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
	if body != "backup command" {
		t.Fatalf("restored body = %q", body)
	}
	attachment, err := os.ReadFile(filepath.Join(restoredDir, "attachments", "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(attachment) != "attachment" {
		t.Fatalf("restored attachment = %q", attachment)
	}

	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "restore", "--from", archive, "--dir", restoredDir,
	}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "not empty") {
		t.Fatalf("non-empty restore: exit %d, stderr:\n%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "restore", "--from", archive, "--dir", restoredDir, "--force",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("force restore exit %d, stderr:\n%s", code, stderr.String())
	}
	safety, err := filepath.Glob(filepath.Join(restoredDir, "backups", "pre-restore-*.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(safety) != 1 {
		t.Fatalf("safety backups = %d, want 1", len(safety))
	}
}

func TestStorageRestoreCommandRequiresAllowNewerSchema(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "gofer.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, storage.CurrentSchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "newer.tar.gz")
	var stdout, stderr bytes.Buffer
	_, code := RunStorageCommand(context.Background(), []string{
		"storage", "backup", "--dir", dataDir, "--db", dbPath, "--out", archive,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("backup exit %d, stderr:\n%s", code, stderr.String())
	}

	restoredDir := filepath.Join(t.TempDir(), "restored")
	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "restore", "--from", archive, "--dir", restoredDir,
	}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "newer than supported") {
		t.Fatalf("newer schema restore: exit %d, stderr:\n%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	_, code = RunStorageCommand(context.Background(), []string{
		"storage", "restore", "--from", archive, "--dir", restoredDir, "--allow-newer-schema",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("allow newer schema exit %d, stderr:\n%s", code, stderr.String())
	}
}

func TestStorageBackupCommandHelpAndUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := RunStorageCommand(context.Background(), []string{"storage", "backup", "--help"}, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("backup help handled %t, exit %d", handled, code)
	}
	if !strings.Contains(stderr.String(), "Usage of storage backup") {
		t.Fatalf("backup help output:\n%s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	handled, code = RunStorageCommand(context.Background(), []string{"storage", "restore"}, &stdout, &stderr)
	if !handled || code != 2 {
		t.Fatalf("restore without --from handled %t, exit %d", handled, code)
	}
	if !strings.Contains(stderr.String(), "usage: gofer storage restore --from PATH") {
		t.Fatalf("restore usage output:\n%s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	handled, code = RunStorageCommand(context.Background(), []string{"storage", "verify"}, &stdout, &stderr)
	if !handled || code != 2 {
		t.Fatalf("verify without --from handled %t, exit %d", handled, code)
	}
	if !strings.Contains(stderr.String(), "usage: gofer storage verify --from PATH") {
		t.Fatalf("verify usage output:\n%s", stderr.String())
	}
}
