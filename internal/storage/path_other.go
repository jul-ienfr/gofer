//go:build !windows

package storage

func canonicalMigrationNativePath(path string) (string, error) {
	return path, nil
}
