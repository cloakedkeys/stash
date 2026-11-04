//go:build windows
// +build windows

package exec

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideExecShell starts the command on its own console, which has no window.
//
// Because the child does not share Stash's console, a Ctrl-C or Ctrl-Break
// sent to Stash does not also reach ffmpeg, ffprobe, plugins or scrapers.
// Processes the child starts itself inherit its hidden console, so they don't
// open windows either.
//
// DETACHED_PROCESS is not used: a detached child has no console at all, so any
// console program it starts gets a new, visible console window. It also
// cancels CREATE_NO_WINDOW. The standard handles are passed explicitly by
// os/exec, so pipes work the same either way.
func hideExecShell(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
