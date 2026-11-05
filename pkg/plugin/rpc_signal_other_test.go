//go:build !windows

package plugin

import (
	"os"
	"os/signal"
)

func ignoreInterrupt() {
	signal.Ignore(os.Interrupt)
}
