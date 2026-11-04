package desktop

import (
	"sync/atomic"
	"testing"
	"time"
)

// A stop requested before the tray is ready must wait for it, then quit it
// and pass the exit code on (#2101).
func TestForwardExitWaitsForTray(t *testing.T) {
	exit := make(chan int)
	ready := make(chan struct{})
	var quits atomic.Int32

	go forwardExit(exit, ready, func() { quits.Add(1) })

	exit <- 3

	time.Sleep(50 * time.Millisecond)
	if n := quits.Load(); n != 0 {
		t.Fatalf("tray quit %d times before it was ready", n)
	}

	close(ready)

	select {
	case code := <-exit:
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exit code was not passed on after the tray was ready")
	}
	if n := quits.Load(); n != 1 {
		t.Errorf("tray quit %d times, want 1", n)
	}
}

// Once the tray is ready, a stop quits it straight away.
func TestForwardExitTrayReady(t *testing.T) {
	exit := make(chan int)
	ready := make(chan struct{})
	close(ready)
	var quits atomic.Int32

	go forwardExit(exit, ready, func() { quits.Add(1) })

	exit <- 0

	select {
	case code := <-exit:
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exit code was not passed on")
	}
	if n := quits.Load(); n != 1 {
		t.Errorf("tray quit %d times, want 1", n)
	}
}
