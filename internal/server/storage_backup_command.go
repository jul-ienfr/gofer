package server

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/backup"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func runStorageBackupCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("storage backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("dir", "", "data directory")
	dbPath := flags.String("db", "", "database path")
	outPath := flags.String("out", "", "archive path")
	overwrite := flags.Bool("force", false, "replace an existing archive")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: gofer storage backup [--dir PATH] [--db PATH] [--out PATH] [--force]")
		return 2
	}
	dir := *dataDir
	if dir == "" {
		dir = filepath.Dir(ConfiguredDatabasePath())
	}
	db := *dbPath
	if db == "" {
		db = filepath.Join(dir, "gofer.db")
	}
	out := *outPath
	if out == "" {
		out = filepath.Join(dir, "backups", "gofer-"+time.Now().UTC().Format("20060102T150405Z")+".tar.gz")
	}
	manifest, err := backup.Create(ctx, backup.CreateOptions{
		DataDir:      dir,
		DatabasePath: db,
		OutputPath:   out,
		Overwrite:    *overwrite,
	})
	if err != nil {
		fmt.Fprintf(stderr, "storage backup failed: %v\n", err)
		return 1
	}
	var size int64
	for _, file := range manifest.Files {
		size += file.Size
	}
	fmt.Fprintf(stdout, "backup created: %s\n", out)
	fmt.Fprintf(stdout, "schema version: %d\n", manifest.SchemaVersion)
	fmt.Fprintf(stdout, "files: %d (%d bytes)\n", len(manifest.Files), size)
	return 0
}

func runStorageVerifyCommand(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("storage verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fromPath := flags.String("from", "", "archive path")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *fromPath == "" {
		fmt.Fprintln(stderr, "usage: gofer storage verify --from PATH")
		return 2
	}
	manifest, err := backup.Verify(*fromPath)
	if err != nil {
		fmt.Fprintf(stderr, "storage verify failed: %v\n", err)
		return 1
	}
	var size int64
	for _, file := range manifest.Files {
		size += file.Size
	}
	fmt.Fprintf(stdout, "backup verified: %s\n", *fromPath)
	fmt.Fprintf(stdout, "schema version: %d\n", manifest.SchemaVersion)
	fmt.Fprintf(stdout, "files: %d (%d bytes)\n", len(manifest.Files), size)
	return 0
}

func runStorageRestoreCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("storage restore", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("dir", "", "data directory")
	fromPath := flags.String("from", "", "archive path")
	force := flags.Bool("force", false, "restore over a non-empty data directory")
	allowNewer := flags.Bool("allow-newer-schema", false, "restore a backup from a newer schema")
	noSafety := flags.Bool("no-safety-backup", false, "skip the automatic pre-restore backup")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *fromPath == "" {
		fmt.Fprintln(stderr, "usage: gofer storage restore --from PATH [--dir PATH] [--force] [--allow-newer-schema] [--no-safety-backup]")
		return 2
	}
	dir := *dataDir
	if dir == "" {
		dir = filepath.Dir(ConfiguredDatabasePath())
	}
	manifest, err := backup.Restore(ctx, *fromPath, backup.RestoreOptions{
		DataDir:          dir,
		Force:            *force,
		AllowNewerSchema: *allowNewer,
		MaxSchemaVersion: storage.CurrentSchemaVersion,
		NoSafetyBackup:   *noSafety,
	})
	if err != nil {
		fmt.Fprintf(stderr, "storage restore failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "backup restored: %s\n", *fromPath)
	fmt.Fprintf(stdout, "data directory: %s\n", dir)
	fmt.Fprintf(stdout, "schema version: %d\n", manifest.SchemaVersion)
	fmt.Fprintf(stdout, "files: %d\n", len(manifest.Files))
	return 0
}
