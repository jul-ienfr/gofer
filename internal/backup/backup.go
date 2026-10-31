package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	FormatVersion  = 1
	manifestName   = "manifest.json"
	maxManifest    = 10 << 20
	maxEntries     = 1_000_000
	maxExtractSize = int64(100 << 30)
)

type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}

type Manifest struct {
	Format        int       `json:"format"`
	CreatedAt     time.Time `json:"created_at"`
	SchemaVersion int       `json:"schema_version"`
	DatabasePath  string    `json:"database_path"`
	Files         []File    `json:"files"`
}

type CreateOptions struct {
	DataDir      string
	DatabasePath string
	OutputPath   string
	Overwrite    bool
}

type RestoreOptions struct {
	DataDir          string
	Force            bool
	AllowNewerSchema bool
	MaxSchemaVersion int
	NoSafetyBackup   bool
}

func Create(ctx context.Context, opts CreateOptions) (*Manifest, error) {
	dataDir, err := filepath.Abs(opts.DataDir)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(dataDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("data directory %s is not usable: %w", dataDir, err)
	}
	dbPath, err := filepath.Abs(opts.DatabasePath)
	if err != nil {
		return nil, err
	}
	dbRel, err := filepath.Rel(dataDir, dbPath)
	if err != nil || dbRel == ".." || strings.HasPrefix(dbRel, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("database %s is outside data directory %s", dbPath, dataDir)
	}
	if info, err := os.Stat(dbPath); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database %s is not a regular file: %w", dbPath, err)
	}
	outPath, err := filepath.Abs(opts.OutputPath)
	if err != nil {
		return nil, err
	}
	if !opts.Overwrite {
		if _, err := os.Stat(outPath); err == nil {
			return nil, fmt.Errorf("backup %s already exists; pass --force to replace it", outPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
		return nil, err
	}

	stage, err := os.MkdirTemp("", "gofer-backup-stage-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)

	files, err := stageData(ctx, dataDir, dbPath, outPath, stage)
	if err != nil {
		return nil, err
	}
	schemaVersion, err := readSchemaVersion(ctx, filepath.Join(stage, filepath.FromSlash(dbRel)))
	if err != nil {
		return nil, fmt.Errorf("read schema version: %w", err)
	}
	manifest := &Manifest{
		Format:        FormatVersion,
		CreatedAt:     time.Now().UTC(),
		SchemaVersion: schemaVersion,
		DatabasePath:  filepath.ToSlash(dbRel),
		Files:         files,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(filepath.Join(stage, manifestName), encoded, 0o600); err != nil {
		return nil, err
	}
	if err := writeArchive(stage, outPath, opts.Overwrite); err != nil {
		return nil, err
	}
	return manifest, nil
}

func Verify(archivePath string) (*Manifest, error) {
	stage, err := os.MkdirTemp("", "gofer-backup-verify-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	manifest, err := extract(archivePath, stage)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func Restore(ctx context.Context, archivePath string, opts RestoreOptions) (*Manifest, error) {
	dataDir, err := filepath.Abs(opts.DataDir)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(dataDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(parent, ".gofer-restore-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	manifest, err := extract(archivePath, stage)
	if err != nil {
		return nil, err
	}
	if opts.MaxSchemaVersion > 0 && manifest.SchemaVersion > opts.MaxSchemaVersion && !opts.AllowNewerSchema {
		return nil, fmt.Errorf("backup schema version %d is newer than supported version %d; pass --allow-newer-schema to override", manifest.SchemaVersion, opts.MaxSchemaVersion)
	}
	if info, err := os.Stat(dataDir); err == nil && !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dataDir)
	}
	nonEmpty, err := hasData(dataDir)
	if err != nil {
		return nil, err
	}
	if nonEmpty && !opts.Force {
		return nil, fmt.Errorf("data directory %s is not empty; pass --force to restore over it", dataDir)
	}
	dbPath, err := safeJoin(dataDir, manifest.DatabasePath)
	if err != nil {
		return nil, err
	}
	if nonEmpty && !opts.NoSafetyBackup {
		if info, err := os.Stat(dbPath); err == nil && info.Mode().IsRegular() {
			safety := filepath.Join(dataDir, "backups", "pre-restore-"+time.Now().UTC().Format("20060102T150405Z")+".tar.gz")
			if _, err := Create(ctx, CreateOptions{DataDir: dataDir, DatabasePath: dbPath, OutputPath: safety}); err != nil {
				return nil, fmt.Errorf("create safety backup: %w", err)
			}
		}
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	for _, file := range manifest.Files {
		src, err := safeJoin(stage, file.Path)
		if err != nil {
			return nil, err
		}
		dst, err := safeJoin(dataDir, file.Path)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		if err := replaceFile(src, dst, fs.FileMode(file.Mode)); err != nil {
			return nil, fmt.Errorf("restore %s: %w", file.Path, err)
		}
		if strings.EqualFold(filepath.Ext(dst), ".db") {
			for _, suffix := range []string{"-wal", "-shm"} {
				if err := os.Remove(dst + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
					return nil, err
				}
			}
		}
	}
	return manifest, nil
}

func stageData(ctx context.Context, dataDir, dbPath, outPath, stage string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(dataDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == dataDir {
			return nil
		}
		rel, err := filepath.Rel(dataDir, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "backups" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		if samePath(abs, outPath) || strings.HasSuffix(rel, "-wal") || strings.HasSuffix(rel, "-shm") || strings.HasSuffix(rel, "-journal") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		dst := filepath.Join(stage, filepath.FromSlash(relSlash))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if ok, err := isSQLite(abs); err != nil {
			return err
		} else if ok {
			if err := vacuumInto(ctx, abs, dst); err != nil {
				return fmt.Errorf("snapshot %s: %w", relSlash, err)
			}
		} else if err := copyFile(dst, abs, info.Mode().Perm()); err != nil {
			return err
		}
		staged, err := os.Stat(dst)
		if err != nil {
			return err
		}
		sum, err := hashFile(dst)
		if err != nil {
			return err
		}
		files = append(files, File{Path: relSlash, Size: staged.Size(), SHA256: sum, Mode: uint32(info.Mode().Perm())})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if len(files) == 0 {
		return nil, errors.New("data directory contains no files to back up")
	}
	found := false
	dbRel, _ := filepath.Rel(dataDir, dbPath)
	for _, file := range files {
		if file.Path == filepath.ToSlash(dbRel) {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("database %s was not included in the backup", dbPath)
	}
	return files, nil
}

func extract(archivePath, destDir string) (*Manifest, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]File{}
	seenNames := map[string]bool{}
	var manifest *Manifest
	var total int64
	entries := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		entries++
		if entries > maxEntries {
			return nil, fmt.Errorf("archive contains more than %d entries", maxEntries)
		}
		name, err := cleanArchivePath(hdr.Name)
		if err != nil {
			return nil, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return nil, fmt.Errorf("archive entry %s has unsupported type %d", name, hdr.Typeflag)
		}
		if seenNames[name] {
			return nil, fmt.Errorf("archive contains duplicate entry %s", name)
		}
		seenNames[name] = true
		if hdr.Size < 0 {
			return nil, fmt.Errorf("archive entry %s has invalid size", name)
		}
		if hdr.Size > maxExtractSize-total {
			return nil, fmt.Errorf("archive expands beyond the %d byte limit", maxExtractSize)
		}
		if name == manifestName && hdr.Size > maxManifest {
			return nil, errors.New("manifest is too large")
		}
		dst, err := safeJoin(destDir, name)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fs.FileMode(hdr.Mode).Perm())
		if err != nil {
			return nil, err
		}
		hasher := sha256.New()
		remaining := maxExtractSize - total
		size, copyErr := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(tr, remaining+1))
		closeErr := out.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if size > remaining {
			return nil, fmt.Errorf("archive expands beyond the %d byte limit", maxExtractSize)
		}
		total += size
		entry := File{Path: name, Size: size, SHA256: hex.EncodeToString(hasher.Sum(nil)), Mode: uint32(hdr.Mode)}
		if name == manifestName {
			if size > maxManifest {
				return nil, errors.New("manifest is too large")
			}
			data, err := os.ReadFile(dst)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("parse manifest: %w", err)
			}
			if manifest.Format != FormatVersion {
				return nil, fmt.Errorf("unsupported backup format %d", manifest.Format)
			}
		} else {
			seen[name] = entry
		}
	}
	if manifest == nil {
		return nil, errors.New("archive has no manifest")
	}
	if len(manifest.Files) == 0 {
		return nil, errors.New("backup manifest lists no files")
	}
	listed := map[string]bool{}
	for _, want := range manifest.Files {
		name, err := cleanArchivePath(want.Path)
		if err != nil {
			return nil, err
		}
		if listed[name] {
			return nil, fmt.Errorf("manifest lists %s more than once", name)
		}
		listed[name] = true
		got, ok := seen[name]
		if !ok {
			return nil, fmt.Errorf("archive is missing %s", name)
		}
		if got.Size != want.Size || !strings.EqualFold(got.SHA256, want.SHA256) {
			return nil, fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	for name := range seen {
		if !listed[name] {
			return nil, fmt.Errorf("archive contains unlisted file %s", name)
		}
	}
	dbName, err := cleanArchivePath(manifest.DatabasePath)
	if err != nil {
		return nil, err
	}
	if !listed[dbName] {
		return nil, fmt.Errorf("manifest database %s is not part of the backup", dbName)
	}
	return manifest, nil
}

func writeArchive(stageDir, outPath string, overwrite bool) error {
	tmp, err := os.CreateTemp(filepath.Dir(outPath), ".gofer-backup-*.tar.gz")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)
	var entries []string
	if err := filepath.WalkDir(stageDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(stageDir, p)
		if err != nil {
			return err
		}
		entries = append(entries, rel)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(entries)
	for _, rel := range entries {
		p := filepath.Join(stageDir, rel)
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if overwrite {
		if err := os.Remove(outPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return err
	}
	return nil
}

func vacuumInto(ctx context.Context, source, target string) error {
	db, err := sql.Open("sqlite", source)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", target); err != nil {
		return err
	}
	return nil
}

func readSchemaVersion(ctx context.Context, dbPath string) (int, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var version int
	err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return 0, nil
	}
	return version, err
}

func isSQLite(p string) (bool, error) {
	file, err := os.Open(p)
	if err != nil {
		return false, err
	}
	defer file.Close()
	var header [16]byte
	n, err := io.ReadFull(file, header[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	return n == len(header) && string(header[:]) == "SQLite format 3\x00", nil
}

func hashFile(p string) (string, error) {
	file, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func copyFile(dst, src string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func replaceFile(src, dst string, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".gofer-restore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(tmp, in)
	closeInErr := in.Close()
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeInErr != nil {
		return closeInErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(tmpName, mode.Perm()); err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpName, dst)
}

func cleanArchivePath(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	return cleaned, nil
}

func safeJoin(root, rel string) (string, error) {
	clean, err := cleanArchivePath(rel)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(rootAbs, filepath.FromSlash(clean))
	check, err := filepath.Rel(rootAbs, joined)
	if err != nil || check == ".." || strings.HasPrefix(check, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes %s", rel, root)
	}
	return joined, nil
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func hasData(dataDir string) (bool, error) {
	info, err := os.Stat(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", dataDir)
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name() == "backups" {
			continue
		}
		return true, nil
	}
	return false, nil
}
