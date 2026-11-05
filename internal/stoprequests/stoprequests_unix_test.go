//go:build !windows

package stoprequests

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
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

// sendSIGTERMToSelf sends SIGTERM to this process, as docker stop and systemd do.
func sendSIGTERMToSelf() error {
	return syscall.Kill(os.Getpid(), syscall.SIGTERM)
}

// SIGTERM, which Docker and systemd send, is caught as a stop request too.
func TestSIGTERMIsCaught(t *testing.T) {
	out, code := runHelper(t, "sigterm")
	assertStoppedCleanly(t, out, code)
}
