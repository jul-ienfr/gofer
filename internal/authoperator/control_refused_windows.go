//go:build windows

package authoperator

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isControlConnectionRefused(err error) bool {
	return errors.Is(err, windows.WSAECONNREFUSED)
}
