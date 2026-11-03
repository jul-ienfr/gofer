//go:build windows

package storage

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func canonicalMigrationNativePath(path string) (string, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	size, err := windows.GetLongPathName(pointer, nil, 0)
	if err != nil {
		return "", err
	}
	if size == 0 {
		return "", fmt.Errorf("resolve long path %q: empty result", path)
	}
	buffer := make([]uint16, size)
	length, err := windows.GetLongPathName(pointer, &buffer[0], size)
	if err != nil {
		return "", err
	}
	if length == 0 || length > size {
		return "", fmt.Errorf("resolve long path %q: invalid result", path)
	}
	return windows.UTF16ToString(buffer[:length]), nil
}
