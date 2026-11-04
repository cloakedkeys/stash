//go:build !windows

package stoprequests

import (
	"os"
	"os/exec"
	"syscall"
)

// setOwnProcessGroup starts cmd in a process group of its own.
func setOwnProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// sendStopToSelf sends SIGINT to this process, as pressing Ctrl-C in its
// terminal would.
func sendStopToSelf() error {
	return syscall.Kill(os.Getpid(), syscall.SIGINT)
}
