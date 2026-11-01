package file

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestDeleter_WindowsNameTooLong(t *testing.T) {
	testDeleterNameTooLong(t, windows.ERROR_FILENAME_EXCED_RANGE)
	testDeleterNameTooLong(t, windows.ERROR_INVALID_NAME)
}
