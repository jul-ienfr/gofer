package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteFileURLUsesAbsoluteURI(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("testdata", "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	got := SQLiteFileURL(path).String()
	if !strings.HasPrefix(got, "file:///") {
		t.Fatalf("SQLiteFileURL(%q) = %q, want file:/// prefix", path, got)
	}
	if strings.HasPrefix(got, "file:C:") {
		t.Fatalf("SQLiteFileURL(%q) = %q, drive parsed as URI authority", path, got)
	}
}

func TestSQLiteFileURLEscapesSpaces(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("testdata", "with space.db"))
	if err != nil {
		t.Fatal(err)
	}
	got := SQLiteFileURL(path).String()
	if strings.Contains(got, " ") {
		t.Fatalf("SQLiteFileURL(%q) = %q, space not escaped", path, got)
	}
}
