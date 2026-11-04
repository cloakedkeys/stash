package desktop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeTray stands in for the systray package. Like the real one, a quit
// before the tray is set up is lost, and run only returns after a quit that
// was not lost.
type fakeTray struct {
	startReady chan struct{} // run calls onReady once this is closed
	initGate   chan struct{} // initialize finishes once this is closed

	mu         sync.Mutex
	initDone   bool
	inits      int
	quits      int
	lostQuits  int
	quitOnce   sync.Once
	quitted    chan struct{}
	runReturns chan struct{}
}

func newFakeTray() *fakeTray {
	return &fakeTray{
		startReady: make(chan struct{}),
		initGate:   make(chan struct{}),
		quitted:    make(chan struct{}),
		runReturns: make(chan struct{}),
	}
}

func (f *fakeTray) run(onReady func()) {
	go func() {
		<-f.startReady
		onReady()
	}()
	<-f.quitted
}

func (f *fakeTray) quit() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quits++
	if !f.initDone {
		f.lostQuits++
		return
	}
	f.quitOnce.Do(func() { close(f.quitted) })
}

func (f *fakeTray) initialize() {
	<-f.initGate
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inits++
	f.initDone = true
}

func (f *fakeTray) counts() (inits, quits, lost int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inits, f.quits, f.lostQuits
}

func (f *fakeTray) start(exit chan int) {
	go func() {
		runTray(exit, f.run, f.quit, f.initialize)
		close(f.runReturns)
	}()
}

func waitExit(t *testing.T, f *fakeTray, exit chan int, want int) {
	t.Helper()
	select {
	case code := <-exit:
		if code != want {
			t.Errorf("exit code = %d, want %d", code, want)
		}
	case <-time.After(5 * time.Second):
		_, quits, lost := f.counts()
		t.Fatalf("exit code was not passed on (quits %d, lost %d)", quits, lost)
	}
	select {
	case <-f.runReturns:
	case <-time.After(5 * time.Second):
		t.Fatal("tray run did not return")
	}
	inits, quits, lost := f.counts()
	if inits != 1 || quits != 1 || lost != 0 {
		t.Errorf("inits %d, quits %d, lost quits %d; want 1, 1, 0", inits, quits, lost)
	}
}

// A stop sent before the tray exists does not quit it until initialize has
// run, then quits it once and passes the code on (#2101).
func TestRunTrayStopBeforeReady(t *testing.T) {
	f := newFakeTray()
	close(f.initGate)
	exit := make(chan int)
	f.start(exit)

	exit <- 3

	time.Sleep(50 * time.Millisecond)
	if _, quits, _ := f.counts(); quits != 0 {
		t.Fatalf("tray quit %d times before it was set up", quits)
	}

	close(f.startReady)
	waitExit(t, f, exit, 3)
}

// Once the tray is set up, a later stop quits it once and passes the code on.
func TestRunTrayStopAfterReady(t *testing.T) {
	f := newFakeTray()
	close(f.initGate)
	close(f.startReady)
	exit := make(chan int)
	f.start(exit)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if inits, _, _ := f.counts(); inits == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tray was not set up")
		}
		time.Sleep(time.Millisecond)
	}

	exit <- 0
	waitExit(t, f, exit, 0)
}

// A stop sent while initialize is still running waits for it to finish.
func TestRunTrayStopDuringInitialize(t *testing.T) {
	f := newFakeTray()
	close(f.startReady)
	exit := make(chan int)
	f.start(exit)

	exit <- 5

	time.Sleep(50 * time.Millisecond)
	if _, quits, _ := f.counts(); quits != 0 {
		t.Fatalf("tray quit %d times while it was being set up", quits)
	}

	close(f.initGate)
	waitExit(t, f, exit, 5)
}

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
