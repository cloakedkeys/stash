package main

import (
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/database/migrationguard"
)

func startEndHelpers(g *migrationguard.Guard) <-chan struct{} {
	ended := make(chan struct{})
	go endHelpers(g, func() { close(ended) })
	return ended
}

// Without a migration, the helpers are ended straight away.
func TestEndHelpersAtOnce(t *testing.T) {
	var g migrationguard.Guard
	select {
	case <-startEndHelpers(&g):
	case <-time.After(5 * time.Second):
		t.Fatal("helpers were not ended")
	}
	if _, err := g.Begin(migrationguard.Update); !errors.Is(err, migrationguard.ErrStopping) {
		t.Errorf("a migration could start after Stash started to exit: %v", err)
	}
}

// A running migration is not cut off: the helpers are ended once it has
// finished, as for a Quit from the tray menu during a database update.
func TestEndHelpersWaitsForMigration(t *testing.T) {
	var g migrationguard.Guard
	end, err := g.Begin(migrationguard.Update)
	if err != nil {
		t.Fatal(err)
	}
	ended := startEndHelpers(&g)

	select {
	case <-ended:
		t.Fatal("helpers were ended while a migration was running")
	case <-time.After(100 * time.Millisecond):
	}

	end()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("helpers were not ended after the migration finished")
	}
}
