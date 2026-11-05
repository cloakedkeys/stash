package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/log"
	"github.com/stashapp/stash/pkg/database/migrationguard"
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

// recordingLogger records the messages logged at Error and Debug level.
type recordingLogger struct {
	logger.LoggerImpl
	errors []string
	debugs []string
}

func (l *recordingLogger) Errorf(format string, args ...interface{}) {
	l.errors = append(l.errors, fmt.Sprintf(format, args...))
}

func (l *recordingLogger) Debugf(format string, args ...interface{}) {
	l.debugs = append(l.debugs, fmt.Sprintf(format, args...))
}

// When startup fails for a reason of its own while a stop comes, such as a
// wrong PostgreSQL password, the error is logged as an error, not hidden. An
// error caused by the stop itself is only logged for debugging.
func TestStoppedDuringStartupLogsRealErrors(t *testing.T) {
	useTestLogger(t)
	rec := &recordingLogger{LoggerImpl: logger.Logger}
	logger.Logger = rec

	stoppedDuringStartup(true, fmt.Errorf("opening the database: %w", context.Canceled))
	stoppedDuringStartup(true, fmt.Errorf("error running initial schema migrations: %w", migrationguard.ErrStopping))
	if len(rec.errors) != 0 {
		t.Errorf("an error caused by the stop should not be logged as an error: %q", rec.errors)
	}
	if len(rec.debugs) != 2 {
		t.Errorf("an error caused by the stop should be logged for debugging, got %q", rec.debugs)
	}

	stoppedDuringStartup(true, errors.New("password authentication failed for user stash"))
	if len(rec.errors) != 1 || !strings.Contains(rec.errors[0], "password authentication failed") {
		t.Errorf("a startup error of its own should be logged as an error, got %q", rec.errors)
	}
}

func TestWaitingMessage(t *testing.T) {
	for _, tc := range []struct {
		kind migrationguard.Kind
		goos string
		want []string
	}{
		{migrationguard.Build, "windows", []string{"database build has finished", "end the Stash process"}},
		{migrationguard.Update, "linux", []string{"database update has finished", "kill -9"}},
		{migrationguard.Update, "darwin", []string{"database update has finished", "kill -9"}},
	} {
		got := waitingMessage(tc.kind, tc.goos)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("waitingMessage(%q, %q) = %q, should contain %q", tc.kind, tc.goos, got, w)
			}
		}
		if strings.Contains(got, "\n") {
			t.Errorf("waitingMessage should be one line, got %q", got)
		}
	}
}
