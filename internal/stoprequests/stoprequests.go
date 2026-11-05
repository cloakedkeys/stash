// Package stoprequests catches the stop requests sent to Stash: Ctrl-C and
// Ctrl-Break on Windows, and SIGINT and SIGTERM elsewhere.
//
// It starts catching them when the package is initialised, before main runs
// and before most other packages are initialised. Until then, a stop request
// ends the process at once, without the clean shutdown (#2103).
//
// A stop request takes effect at once, unless a database migration is
// running. Then it takes effect once the migration has finished, so the
// migration isn't cut off halfway.
package stoprequests

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/stashapp/stash/pkg/database/migrationguard"
)

// guard is what stop requests wait for: the running database migrations.
type guard interface {
	TryStop() bool
	Stop(waiting func())
}

// stopSignals are the signals caught as stop requests. Docker and systemd send
// SIGTERM; Ctrl-C and Ctrl-Break arrive as SIGINT.
var stopSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

var (
	signals = make(chan os.Signal, 1)
	stopped = make(chan struct{})

	// onWaiting is called for each stop request that has to wait for a
	// migration to finish.
	onWaiting atomic.Pointer[func()]
)

// init starts catching stop requests as early as possible.
func init() {
	signal.Notify(signals, stopSignals...)
	go dispatch(signals, migrationguard.Default(), closeStopped, waiting)
}

var stopOnce sync.Once

func closeStopped() { stopOnce.Do(func() { close(stopped) }) }

func waiting() {
	if f := onWaiting.Load(); f != nil {
		(*f)()
	}
}

// dispatch handles the stop requests from signals. The first one calls stop,
// at once if g has no migration running, otherwise once the migrations have
// finished. Each request that comes while a migration is running calls
// waiting. Requests after stop has been called are ignored, as before #2103.
func dispatch(signals <-chan os.Signal, g guard, stop func(), waiting func()) {
	done := false
	var held chan struct{}

	for {
		select {
		case <-signals:
			switch {
			case done:
			case held != nil:
				waiting()
			case g.TryStop():
				done = true
				stop()
			default:
				waiting()
				held = make(chan struct{})
				go func(held chan struct{}) {
					g.Stop(nil)
					close(held)
				}(held)
			}
		case <-held:
			held = nil
			done = true
			stop()
		}
	}
}

// OnWaiting sets the function called for each stop request that has to wait
// for a database migration to finish.
func OnWaiting(f func()) {
	onWaiting.Store(&f)
}

// Stopped is closed once a stop request has taken effect.
func Stopped() <-chan struct{} {
	return stopped
}

// Requested reports whether a stop request has taken effect.
func Requested() bool {
	select {
	case <-stopped:
		return true
	default:
		return false
	}
}

// Forward waits until a stop request has taken effect, then sends the exit
// code of a normal stop, 0, on exit.
func Forward(exit chan<- int) {
	<-stopped
	exit <- 0
}

// Release stops catching stop requests, so that the next one ends the
// process at once again, as it did before #2103.
func Release() {
	signal.Stop(signals)
}
