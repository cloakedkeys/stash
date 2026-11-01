//go:build !windows

package file

import (
	"errors"
	"syscall"
)

func isDeleteNameTooLong(err error) bool {
	return errors.Is(err, syscall.ENAMETOOLONG)
}
