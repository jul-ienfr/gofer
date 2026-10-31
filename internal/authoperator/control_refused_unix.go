//go:build !windows

package authoperator

import (
	"errors"
	"syscall"
)

func isControlConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
