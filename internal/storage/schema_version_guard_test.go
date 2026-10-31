package storage

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "newer.db")
	db, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Write().Exec(`DELETE FROM schema_version`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Write().Exec(`INSERT INTO schema_version (version) VALUES (?)`, CurrentSchemaVersion+1); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := New(path); err == nil || !strings.Contains(err.Error(), "newer Gofer release") {
		t.Fatalf("New() error = %v, want newer-release rejection", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var version int
	if err := raw.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion+1 {
		t.Fatalf("schema version changed to %d, want %d", version, CurrentSchemaVersion+1)
	}
}
