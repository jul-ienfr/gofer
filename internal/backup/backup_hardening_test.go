package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type hardeningEntry struct {
	name string
	data []byte
}

func writeHardeningArchive(t *testing.T, entries []hardeningEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     0o600,
			Size:     int64(len(entry.data)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(entry.data); err != nil {
			t.Fatal(err)
		}
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
	return path
}

func hardeningManifest(t *testing.T, manifest Manifest) []byte {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRestoreRejectsUnsafeDatabasePath(t *testing.T) {
	archive := writeHardeningArchive(t, []hardeningEntry{
		{name: manifestName, data: hardeningManifest(t, Manifest{
			Format:       FormatVersion,
			DatabasePath: "../outside.db",
		})},
	})
	workDir := t.TempDir()
	dataDir := filepath.Join(workDir, "data")
	if _, err := Restore(context.Background(), archive, RestoreOptions{DataDir: dataDir, Force: true}); err == nil {
		t.Fatal("expected restore to reject unsafe database path")
	}
	if _, err := os.Stat(filepath.Join(workDir, "outside.db")); !os.IsNotExist(err) {
		t.Fatalf("outside database path exists: %v", err)
	}
}

func TestRestoreRejectsDuplicateArchiveEntries(t *testing.T) {
	manifest := hardeningManifest(t, Manifest{
		Format:       FormatVersion,
		DatabasePath: "gofer.db",
	})
	archive := writeHardeningArchive(t, []hardeningEntry{
		{name: manifestName, data: manifest},
		{name: "data.txt", data: []byte("first")},
		{name: "data.txt", data: []byte("second")},
	})
	resp, err := Restore(context.Background(), archive, RestoreOptions{DataDir: t.TempDir(), Force: true})
	if err == nil {
		t.Fatalf("expected duplicate entry error, got manifest %#v", resp)
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyRejectsChecksumMismatch(t *testing.T) {
	archive := writeHardeningArchive(t, []hardeningEntry{
		{name: manifestName, data: hardeningManifest(t, Manifest{
			Format:       FormatVersion,
			DatabasePath: "gofer.db",
			Files: []File{{
				Path:   "gofer.db",
				Size:   9,
				SHA256: "deadbeef",
			}},
		})},
		{name: "gofer.db", data: []byte("not a db!")},
	})
	if _, err := Verify(archive); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestRestoreRejectsUnlistedArchiveFile(t *testing.T) {
	data := []byte("not a db!")
	sum := sha256.Sum256(data)
	archive := writeHardeningArchive(t, []hardeningEntry{
		{name: manifestName, data: hardeningManifest(t, Manifest{
			Format:       FormatVersion,
			DatabasePath: "gofer.db",
			Files: []File{{
				Path:   "gofer.db",
				Size:   int64(len(data)),
				SHA256: hex.EncodeToString(sum[:]),
			}},
		})},
		{name: "gofer.db", data: data},
		{name: "extra.txt", data: []byte("extra")},
	})
	if _, err := Verify(archive); err == nil || !strings.Contains(err.Error(), "unlisted file") {
		t.Fatalf("expected unlisted file rejection, got %v", err)
	}
}

func TestRestoreRejectsOversizedManifest(t *testing.T) {
	archive := writeHardeningArchive(t, []hardeningEntry{
		{name: manifestName, data: bytes.Repeat([]byte(" "), maxManifest+1)},
	})
	if _, err := Restore(context.Background(), archive, RestoreOptions{DataDir: t.TempDir(), Force: true}); err == nil {
		t.Fatal("expected oversized manifest rejection")
	}
}
