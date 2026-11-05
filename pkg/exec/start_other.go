//go:build !windows
// +build !windows

package exec

import "os/exec"

// Outside Windows the exec.Cmd methods are used unchanged.

func start(cmd *exec.Cmd) error {
	return cmd.Start()
}

func run(cmd *exec.Cmd) error {
	return cmd.Run()
}

func output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func combinedOutput(cmd *exec.Cmd) ([]byte, error) {
	return cmd.CombinedOutput()
}
