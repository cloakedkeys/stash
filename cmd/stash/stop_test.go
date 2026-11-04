package main

import (
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/log"
	"github.com/stashapp/stash/pkg/logger"
)

func useTestLogger(t *testing.T) {
	t.Helper()
	origCode := exitCode
	origLogger := logger.Logger
	t.Cleanup(func() {
		exitCode = origCode
		logger.Logger = origLogger
	})
	l := log.NewLogger()
	l.Init("", true, "Error", 0)
	logger.Logger = l
	exitCode = 0
}

// After each startup step, main stops if a stop request has taken effect,
// whether or not the step failed because of it, and goes on otherwise.
func TestStoppedDuringStartup(t *testing.T) {
	useTestLogger(t)
	stepErr := errors.New("opening the database: context canceled")

	for _, tc := range []struct {
		name      string
		requested bool
		err       error
		want      bool
	}{
		{"no stop", false, nil, false},
		{"no stop, step failed", false, stepErr, false},
		{"stop", true, nil, true},
		{"stop ended the step", true, stepErr, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exitCode = 0
			if got := stoppedDuringStartup(tc.requested, tc.err); got != tc.want {
				t.Errorf("stoppedDuringStartup(%v, %v) = %v, want %v", tc.requested, tc.err, got, tc.want)
			}
			if exitCode != 0 {
				t.Errorf("a stop during startup should leave the exit code of a normal stop, 0, got %d", exitCode)
			}
		})
	}
}

// The copy to PostgreSQL exits with 0 when it finished, and with 1 when it
// failed or a stop ended it, as a stop that ended it before #2103 didn't give 0.
func TestFinishCopy(t *testing.T) {
	useTestLogger(t)
	copyErr := errors.New("copying: context canceled")

	for _, tc := range []struct {
		name      string
		requested bool
		err       error
		want      int
	}{
		{"copied", false, nil, 0},
		{"copied, then stopped", true, nil, 0},
		{"failed", false, copyErr, 1},
		{"stopped", true, copyErr, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exitCode = 0
			finishCopy(tc.requested, tc.err)
			if exitCode != tc.want {
				t.Errorf("exit code %d, want %d", exitCode, tc.want)
			}
		})
	}
}

func TestContextUntil(t *testing.T) {
	done := make(chan struct{})
	ctx, cancel := contextUntil(done)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("the context was cancelled before done was closed")
	case <-time.After(20 * time.Millisecond):
	}

	close(done)
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the context was not cancelled once done was closed")
	}
}
