package ffmpeg

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
)

func TestIsAlreadyExitedKillError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"other error", errors.New("ffmpeg failed"), false},
		{"access denied", os.NewSyscallError("TerminateProcess", syscall.EACCES), true},
		{"access denied wrapped by exec", fmt.Errorf("exec: canceling Cmd: %w", os.NewSyscallError("TerminateProcess", syscall.EACCES)), true},
		{"other kill failure", os.NewSyscallError("TerminateProcess", syscall.EINVAL), false},
		{"access denied elsewhere", os.NewSyscallError("DuplicateHandle", syscall.EACCES), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAlreadyExitedKillError(tt.err); got != tt.want {
				t.Errorf("isAlreadyExitedKillError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
