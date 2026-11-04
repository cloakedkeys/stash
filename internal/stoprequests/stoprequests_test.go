package stoprequests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPending(t *testing.T) {
	r := make(Requests, 1)
	if r.Pending() {
		t.Fatal("Pending should be false before a stop request arrives")
	}

	r <- os.Interrupt
	if !r.Pending() {
		t.Fatal("Pending should be true after a stop request arrives")
	}
	if r.Pending() {
		t.Error("Pending should use up the request it reports")
	}
}

func TestForward(t *testing.T) {
	r := make(Requests, 1)
	exit := make(chan int)
	go r.Forward(exit)

	select {
	case code := <-exit:
		t.Fatalf("Forward sent exit code %d before any stop request", code)
	case <-time.After(50 * time.Millisecond):
	}

	r <- os.Interrupt
	select {
	case code := <-exit:
		if code != 0 {
			t.Errorf("a stop request should give the exit code of a normal stop, 0, got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Forward did not pass on the stop request")
	}
}

func TestForwardEarlyRequest(t *testing.T) {
	// a stop that arrived before Forward started is passed on too
	r := make(Requests, 1)
	r <- os.Interrupt
	exit := make(chan int)
	go r.Forward(exit)

	select {
	case code := <-exit:
		if code != 0 {
			t.Errorf("expected exit code 0, got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Forward did not pass on the stop request that came first")
	}
}

const helperEnv = "STASH_TEST_STOPREQUESTS_HELPER"

// TestHelperProcess is not a real test. The tests below run it in a separate
// process, which sends a stop request to itself while it is "starting up".
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("only runs as a helper process")
	}

	r := Caught()
	switch mode {
	case "reset":
		// nothing catches stop requests, as before #2103
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	case "release":
		r.Release()
	}

	if err := sendStopToSelf(); err != nil {
		fmt.Println("could not send stop request:", err)
		os.Exit(2)
	}

	// startup work, which the stop request must not cut off
	time.Sleep(200 * time.Millisecond)
	fmt.Println("startup finished")

	select {
	case <-r:
		fmt.Println("stopping cleanly")
		os.Exit(0)
	case <-time.After(10 * time.Second):
		fmt.Println("stop request never arrived")
		os.Exit(3)
	}
}

func runHelper(t *testing.T, mode string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	setOwnProcessGroup(cmd)
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running helper: %v", err)
	}
	t.Logf("helper (%s) exited with code 0x%x", mode, uint32(code))
	return string(out), code
}

// A stop request that comes while Stash is starting up is caught without any
// call from main: it waits until startup has finished, and then stops Stash
// with the exit code of a normal stop (#2103).
func TestStopDuringStartupIsCaught(t *testing.T) {
	out, code := runHelper(t, "caught")
	if code != 0 {
		t.Errorf("helper should exit with code 0, got %d (0x%x); output:\n%s", code, uint32(code), out)
	}
	if !strings.Contains(out, "startup finished") || !strings.Contains(out, "stopping cleanly") {
		t.Errorf("helper should finish starting up and then stop cleanly; output:\n%s", out)
	}
}

// When nothing catches it, the same stop request ends the process at once.
// This is what Stash did before #2103, and shows that the test above sends a
// real stop request.
func TestStopDuringStartupUncaught(t *testing.T) {
	out, code := runHelper(t, "reset")
	assertEndedAtOnce(t, out, code)
}

// After Release, a stop request ends the process at once again, as the
// SQLite to PostgreSQL copy expects.
func TestStopAfterRelease(t *testing.T) {
	out, code := runHelper(t, "release")
	assertEndedAtOnce(t, out, code)
}

func assertEndedAtOnce(t *testing.T, out string, code int) {
	t.Helper()
	if code == 0 || code == 2 || code == 3 || strings.Contains(out, "startup finished") {
		t.Errorf("the stop request should end the helper at once; exit code %d (0x%x), output:\n%s", code, uint32(code), out)
	}
}
