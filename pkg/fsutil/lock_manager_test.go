package fsutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestReadLockCommandHelper(t *testing.T) {
	if os.Getenv("STASH_READ_LOCK_COMMAND_HELPER") != "1" {
		return
	}
	fmt.Println("ready")
	time.Sleep(time.Minute)
	os.Exit(0)
}

type commandReadyWriter struct {
	ready chan struct{}
	once  sync.Once
}

func (w *commandReadyWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.ready) })
	return len(p), nil
}

func startReadLockCommand(t *testing.T, ctx context.Context) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReadLockCommandHelper$")
	cmd.Env = append(os.Environ(), "STASH_READ_LOCK_COMMAND_HELPER=1")
	w := &commandReadyWriter{ready: make(chan struct{})}
	// A writer rather than an *os.File exercises Cmd.Wait's output-copy wait,
	// as generation does when it captures ffmpeg's output.
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	select {
	case <-w.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("command did not start")
	}
	return cmd
}

func TestReadLockCancelSharesCommandWait(t *testing.T) {
	m := NewReadLockManager()
	ctx := m.ReadLock(context.Background(), "scene.mp4")
	defer ctx.Cancel()
	cmd := startReadLockCommand(t, ctx)
	wait := ctx.AttachCommand(cmd)

	// Deletion can win the race to wait for the encoder. The task must still
	// receive its actual exit result, rather than calling Cmd.Wait a second time.
	m.Cancel("scene.mp4")
	err := wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait after deletion = %v, want process exit error", err)
	}
	if again := wait(); again != err {
		t.Fatalf("repeated wait = %v, want the same exit result %v", again, err)
	}
}

func TestReadLockCancelWhileCommandWaits(t *testing.T) {
	m := NewReadLockManager()
	ctx := m.ReadLock(context.Background(), "scene.mp4")
	defer ctx.Cancel()
	wait := ctx.AttachCommand(startReadLockCommand(t, ctx))

	finished := make(chan error, 1)
	go func() { finished <- wait() }()
	m.Cancel("scene.mp4")
	select {
	case err := <-finished:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("cancelled command = %v, want process exit error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("command owner remained blocked after deletion")
	}
}
