package file

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func isDeleteNameTooLong(err error) bool {
	// The existing name is valid and ".delete" adds no invalid characters, so ERROR_INVALID_NAME means it is too long.
	return errors.Is(err, windows.ERROR_FILENAME_EXCED_RANGE) || errors.Is(err, windows.ERROR_INVALID_NAME) || errors.Is(err, syscall.ENAMETOOLONG)
}
