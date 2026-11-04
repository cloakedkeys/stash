// Package stoprequests catches the stop requests sent to Stash: Ctrl-C and
// Ctrl-Break on Windows, and SIGINT and SIGTERM elsewhere.
//
// It starts catching them when the package is initialised, before main runs
// and before most other packages are initialised. Until then, a stop request
// ends the process at once, without the clean shutdown (#2103).
package stoprequests

import (
	"os"
	"os/signal"
	"syscall"
)

// Requests receives stop requests.
type Requests chan os.Signal

// caught is set when the package is initialised, so that stop requests are
// caught as early as possible.
var caught = catch()

func catch() Requests {
	r := make(Requests, 1)
	signal.Notify(r, syscall.SIGINT, syscall.SIGTERM)
	return r
}

// Caught returns the stop requests caught since the package was initialised.
func Caught() Requests {
	return caught
}

// Pending reports whether a stop request has arrived, without waiting for
// one. A request it reports is used up.
func (r Requests) Pending() bool {
	select {
	case <-r:
		return true
	default:
		return false
	}
}

// Forward waits for a stop request, then sends the exit code of a normal
// stop, 0, on exit.
func (r Requests) Forward(exit chan<- int) {
	<-r
	exit <- 0
}

// Release stops catching stop requests, so that the next one ends the
// process at once again.
func (r Requests) Release() {
	signal.Stop(r)
}
