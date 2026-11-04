//go:build windows

package stoprequests

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// setOwnProcessGroup starts cmd in a process group of its own, so that the
// Ctrl-Break it sends itself doesn't reach the test.
func setOwnProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// sendStopToSelf sends a Ctrl-Break to this process's own process group, as
// pressing Ctrl-Break in its console would.
func sendStopToSelf() error {
	// Ctrl events need a console. If this process has one already this fails,
	// which is fine; otherwise the new console holds only this process.
	_, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("AllocConsole").Call()
	_ = err
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(os.Getpid()))
}
