//go:build windows

package exec

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// waitGone reports whether the process exits within timeout.
func waitGone(pid int, timeout time.Duration) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true // already gone
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	ev, _ := windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	return ev == windows.WAIT_OBJECT_0
}

func killPid(pid int) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	_ = windows.TerminateProcess(h, 1)
	_ = windows.CloseHandle(h)
}

// A helper and everything it starts end when the process that started it
// exits, without being stopped.
func TestHelpersEndWhenOwnerExits(t *testing.T) {
	// The owner is started with plain os/exec, so that the job it creates is
	// its own and not this test's.
	owner := exec.Command(helperCommand(context.Background(), "owner").Path, "-test.run=^TestHelperProcess$")
	owner.Env = helperCommand(context.Background(), "owner").Env
	out, err := owner.Output()
	if err != nil {
		t.Fatalf("owner failed: %v\n%s", err, out)
	}
	var child, grandchild int
	if _, err := fmt.Sscan(string(out), &child, &grandchild); err != nil {
		t.Fatalf("bad owner output %q: %v", out, err)
	}
	t.Cleanup(func() { killPid(child); killPid(grandchild) })

	if !waitGone(child, 5*time.Second) {
		t.Errorf("helper %d still running after its owner exited", child)
	}
	if !waitGone(grandchild, 5*time.Second) {
		t.Errorf("helper's own child %d still running after the owner exited", grandchild)
	}
}

// Cancelling the context stops only the helper itself, as exec.CommandContext
// does. Its own children are left to the job.
func TestCancelStopsOnlyDirectChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := helperCommand(ctx, "spawn")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	var grandchild int
	if _, err := fmt.Fscan(bufio.NewReader(stdout), &grandchild); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killPid(grandchild) })

	cancel()
	if err := cmd.Wait(); err == nil {
		t.Errorf("expected an error from a cancelled command")
	}
	if waitGone(grandchild, 500*time.Millisecond) {
		t.Errorf("cancelling stopped the helper's own child too")
	}
}

func TestStartReturnsStartErrors(t *testing.T) {
	cmd := Command(`C:\does\not\exist.exe`)
	if err := Start(cmd); err == nil {
		t.Fatal("expected an error")
	}
	if cmd.Process != nil {
		t.Error("process set after a failed start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd = helperCommand(ctx, "sleep")
	if err := Start(cmd); !errors.Is(err, context.Canceled) {
		t.Errorf("start with a cancelled context: got %v, want context.Canceled", err)
	}
}

func TestStartKeepsCreationFlags(t *testing.T) {
	cmd := helperCommand(context.Background(), "echo")
	before := cmd.SysProcAttr.CreationFlags
	if _, err := Output(cmd); err == nil {
		t.Fatal("expected exit status 3")
	}
	if cmd.SysProcAttr.CreationFlags != before {
		t.Errorf("creation flags changed from %#x to %#x", before, cmd.SysProcAttr.CreationFlags)
	}
}

func TestOutputMatchesCmdOutput(t *testing.T) {
	ours, ourErr := Output(helperCommand(context.Background(), "fail"))
	theirs, theirErr := helperCommand(context.Background(), "fail").Output()

	if string(ours) != string(theirs) || string(ours) != "partial" {
		t.Errorf("stdout %q, os/exec gives %q", ours, theirs)
	}
	var ourExit, theirExit *exec.ExitError
	if !errors.As(ourErr, &ourExit) || !errors.As(theirErr, &theirExit) {
		t.Fatalf("errors %v and %v, want exit errors", ourErr, theirErr)
	}
	if ourExit.ExitCode() != 2 || theirExit.ExitCode() != 2 {
		t.Errorf("exit codes %d and %d, want 2", ourExit.ExitCode(), theirExit.ExitCode())
	}
	if string(ourExit.Stderr) != string(theirExit.Stderr) || string(ourExit.Stderr) != "something went wrong" {
		t.Errorf("ExitError.Stderr %q, os/exec gives %q", ourExit.Stderr, theirExit.Stderr)
	}

	// Stderr set by the caller is used, and not copied into the error.
	cmd := helperCommand(context.Background(), "fail")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_, err := Output(cmd)
	if !errors.As(err, &ourExit) || ourExit.Stderr != nil || stderr.String() != "something went wrong" {
		t.Errorf("with Stderr set: err %v, ExitError.Stderr %q, Stderr %q", err, ourExit.Stderr, stderr.String())
	}

	cmd = helperCommand(context.Background(), "fail")
	cmd.Stdout = &bytes.Buffer{}
	if _, err := Output(cmd); err == nil || err.Error() != "exec: Stdout already set" {
		t.Errorf("with Stdout set: got %v", err)
	}
}

func TestCombinedOutputMatchesCmdCombinedOutput(t *testing.T) {
	ours, ourErr := CombinedOutput(helperCommand(context.Background(), "fail"))
	theirs, theirErr := helperCommand(context.Background(), "fail").CombinedOutput()
	if string(ours) != string(theirs) || !strings.Contains(string(ours), "partial") || !strings.Contains(string(ours), "something went wrong") {
		t.Errorf("output %q, os/exec gives %q", ours, theirs)
	}
	var ourExit, theirExit *exec.ExitError
	if !errors.As(ourErr, &ourExit) || !errors.As(theirErr, &theirExit) || ourExit.ExitCode() != theirExit.ExitCode() {
		t.Errorf("errors %v and %v", ourErr, theirErr)
	}

	cmd := helperCommand(context.Background(), "fail")
	cmd.Stdout = &bytes.Buffer{}
	if _, err := CombinedOutput(cmd); err == nil || err.Error() != "exec: Stdout already set" {
		t.Errorf("with Stdout set: got %v", err)
	}
	cmd = helperCommand(context.Background(), "fail")
	cmd.Stderr = &bytes.Buffer{}
	if _, err := CombinedOutput(cmd); err == nil || err.Error() != "exec: Stderr already set" {
		t.Errorf("with Stderr set: got %v", err)
	}
}

func TestRunReportsExitCode(t *testing.T) {
	err := Run(helperCommand(context.Background(), "fail"))
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Errorf("got %v, want exit status 2", err)
	}
	if err := Run(helperCommand(context.Background(), "grandchild")); err != nil {
		t.Errorf("got %v, want success", err)
	}
}

func TestHeadTailBuffer(t *testing.T) {
	b := &headTailBuffer{n: 4}
	_, _ = b.Write([]byte("ab"))
	if got := string(b.Bytes()); got != "ab" {
		t.Errorf("got %q", got)
	}
	_, _ = b.Write([]byte("cdef"))
	if got := string(b.Bytes()); got != "abcdef" {
		t.Errorf("got %q", got)
	}
	_, _ = b.Write([]byte("ghijk"))
	if got := string(b.Bytes()); got != "abcd\n... omitting 3 bytes ...\nhijk" {
		t.Errorf("got %q", got)
	}
}

// StopAll ends running helpers and their own children, and helpers can still
// be started afterwards.
func TestStopAll(t *testing.T) {
	cmd := helperCommand(context.Background(), "spawn")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := Start(cmd); err != nil {
		t.Fatal(err)
	}
	var grandchild int
	if _, err := fmt.Fscan(bufio.NewReader(stdout), &grandchild); err != nil {
		t.Fatal(err)
	}
	child := cmd.Process.Pid
	t.Cleanup(func() { killPid(child); killPid(grandchild) })

	StopAll()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			t.Errorf("helper ended with %v, want exit status 1", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StopAll did not stop the helper")
	}
	if !waitGone(grandchild, 5*time.Second) {
		t.Error("StopAll did not stop the helper's own child")
	}

	if err := Run(helperCommand(context.Background(), "grandchild")); err != nil {
		t.Errorf("helper started after StopAll failed: %v", err)
	}
}
