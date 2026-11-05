// Package exec provides functions that wrap os/exec functions. These functions
// prevent external commands from opening windows on the Windows platform, and
// on Windows make sure the commands, and anything they start, end when Stash
// exits.
//
// Commands made with Command or CommandContext must be started with this
// package's Start, Run, Output or CombinedOutput, not the methods of the same
// name on exec.Cmd. On other platforms these are the exec.Cmd methods.
package exec

import (
	"context"
	"os/exec"
)

// Command wraps the exec.Command function, preventing Windows from opening a window when starting.
func Command(name string, arg ...string) *exec.Cmd {
	ret := exec.Command(name, arg...)
	hideExecShell(ret)
	return ret
}

// CommandContext wraps the exec.CommandContext function, preventing Windows from opening a window when starting.
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	ret := exec.CommandContext(ctx, name, arg...)
	hideExecShell(ret)
	return ret
}

// Start starts cmd, like cmd.Start. On Windows the started process is also
// added to a job that ends it, and everything it starts, when Stash exits.
func Start(cmd *exec.Cmd) error {
	return start(cmd)
}

// Run starts cmd and waits for it to finish, like cmd.Run.
func Run(cmd *exec.Cmd) error {
	return run(cmd)
}

// Output runs cmd and returns its standard output, like cmd.Output.
func Output(cmd *exec.Cmd) ([]byte, error) {
	return output(cmd)
}

// CombinedOutput runs cmd and returns its combined standard output and
// standard error, like cmd.CombinedOutput.
func CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	return combinedOutput(cmd)
}

// StopAll ends every process started through this package that is still
// running, and everything those processes started. Stash calls it when it
// begins to exit, so that requests and jobs waiting on a helper finish
// instead of holding up the exit. It does nothing outside Windows.
func StopAll() {
	stopAll()
}
