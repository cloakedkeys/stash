package stoprequests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/database/migrationguard"
)

func TestStopSignals(t *testing.T) {
	// Ctrl-C and Ctrl-Break arrive as SIGINT; Docker and systemd send SIGTERM
	for _, want := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		found := false
		for _, s := range stopSignals {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%v is not caught as a stop request", want)
		}
	}
}

// startDispatch runs dispatch on a guard of its own, and returns the channel
// to send stop requests on, a channel closed when the stop takes effect, and
// the number of calls to waiting so far.
func startDispatch(t *testing.T, g *migrationguard.Guard) (chan<- os.Signal, <-chan struct{}, *atomic.Int32) {
	t.Helper()
	sigs := make(chan os.Signal)
	stopped := make(chan struct{})
	var waits atomic.Int32
	go dispatch(sigs, g, func() { close(stopped) }, func() { waits.Add(1) })
	return sigs, stopped, &waits
}

func assertStopped(t *testing.T, stopped <-chan struct{}, want bool) {
	t.Helper()
	wait := 50 * time.Millisecond
	if want {
		wait = 5 * time.Second
	}
	select {
	case <-stopped:
		if !want {
			t.Fatal("the stop took effect while a migration was running")
		}
	case <-time.After(wait):
		if want {
			t.Fatal("the stop did not take effect")
		}
	}
}

func TestDispatchStopsAtOnceWithoutMigration(t *testing.T) {
	var g migrationguard.Guard
	sigs, stopped, waits := startDispatch(t, &g)

	sigs <- os.Interrupt
	assertStopped(t, stopped, true)
	if n := waits.Load(); n != 0 {
		t.Errorf("waiting was called %d times without a migration", n)
	}

	// later requests are ignored, as before #2103; a second stop must not panic
	sigs <- os.Interrupt
	if _, err := g.Begin(migrationguard.Update); !errors.Is(err, migrationguard.ErrStopping) {
		t.Errorf("a migration could start after the stop took effect: %v", err)
	}
}

func TestDispatchWaitsForMigration(t *testing.T) {
	var g migrationguard.Guard
	end, err := g.Begin(migrationguard.Update)
	if err != nil {
		t.Fatal(err)
	}
	sigs, stopped, waits := startDispatch(t, &g)

	sigs <- os.Interrupt
	assertStopped(t, stopped, false)

	// a second stop during the migration only says that Stash will stop once
	// it has finished
	sigs <- os.Interrupt
	sigs <- syscall.SIGTERM
	assertStopped(t, stopped, false)
	if n := waits.Load(); n != 3 {
		t.Errorf("each stop during the migration should call waiting, got %d calls for 3 stops", n)
	}

	end()
	assertStopped(t, stopped, true)
}

const helperEnv = "STASH_TEST_STOPREQUESTS_HELPER"

// TestHelperProcess is not a real test. The tests below run it in a separate
// process, which sends a stop request to itself while it is "starting up".
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		t.Skip("only runs as a helper process")
	}

	OnWaiting(func() { fmt.Println("waiting for the migration") })

	send := sendStopToSelf
	switch mode {
	case "reset":
		// nothing catches stop requests, as before #2103
		signal.Reset(stopSignals...)
	case "release":
		Release()
	case "sigterm":
		send = sendSIGTERMToSelf
	case "migration":
		end, err := migrationguard.Begin(migrationguard.Build)
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		send = func() error {
			if err := sendStopToSelf(); err != nil {
				return err
			}
			time.Sleep(100 * time.Millisecond)
			// a second stop must not cut the migration off either
			if err := sendStopToSelf(); err != nil {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			if Requested() {
				fmt.Println("stopped during the migration")
				os.Exit(5)
			}
			fmt.Println("migration finished")
			end()
			return nil
		}
	}

	if err := send(); err != nil {
		fmt.Println("could not send stop request:", err)
		os.Exit(2)
	}

	// startup work, which a stop request outside a migration doesn't wait for
	time.Sleep(100 * time.Millisecond)
	fmt.Println("startup step finished")

	select {
	case <-Stopped():
		fmt.Println("stopping cleanly")
		os.Exit(0)
	case <-time.After(10 * time.Second):
		fmt.Println("stop request never took effect")
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

func assertStoppedCleanly(t *testing.T, out string, code int) {
	t.Helper()
	if code != 0 || !strings.Contains(out, "stopping cleanly") {
		t.Errorf("helper should stop cleanly with code 0, got %d (0x%x); output:\n%s", code, uint32(code), out)
	}
}

// A stop request that comes while Stash is starting up is caught without any
// call from main, and takes effect at once (#2103).
func TestStopDuringStartupIsCaught(t *testing.T) {
	out, code := runHelper(t, "caught")
	assertStoppedCleanly(t, out, code)
	if strings.Contains(out, "waiting for the migration") {
		t.Errorf("a stop without a migration should not wait; output:\n%s", out)
	}
}

// A stop request during a migration waits for it to finish, however many
// come (#2103).
func TestStopDuringMigrationWaits(t *testing.T) {
	out, code := runHelper(t, "migration")
	assertStoppedCleanly(t, out, code)
	if !strings.Contains(out, "migration finished") {
		t.Errorf("the migration should finish before the stop; output:\n%s", out)
	}
	if n := strings.Count(out, "waiting for the migration"); n != 2 {
		t.Errorf("each of the 2 stops during the migration should say Stash is waiting, got %d; output:\n%s", n, out)
	}
}

// When nothing catches it, the same stop request ends the process at once.
// This is what Stash did before #2103, and shows that the tests above send a
// real stop request.
func TestStopDuringStartupUncaught(t *testing.T) {
	out, code := runHelper(t, "reset")
	assertEndedAtOnce(t, out, code)
}

// After Release, a stop request ends the process at once again, as the
// --users and --reset-password commands expect.
func TestStopAfterRelease(t *testing.T) {
	out, code := runHelper(t, "release")
	assertEndedAtOnce(t, out, code)
}

func assertEndedAtOnce(t *testing.T, out string, code int) {
	t.Helper()
	if code == 0 || code == 2 || code == 3 || strings.Contains(out, "startup step finished") {
		t.Errorf("the stop request should end the helper at once; exit code %d (0x%x), output:\n%s", code, uint32(code), out)
	}
}
