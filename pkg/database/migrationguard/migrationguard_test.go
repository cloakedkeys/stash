package migrationguard

import (
	"errors"
	"testing"
	"time"
)

func TestTryStopWithoutMigration(t *testing.T) {
	var g Guard
	if g.Running() {
		t.Fatal("a new guard has no migration running")
	}
	if !g.TryStop() {
		t.Fatal("TryStop should succeed when no migration is running")
	}
	if _, err := g.Begin(Update); !errors.Is(err, ErrStopping) {
		t.Errorf("Begin after a stop should return ErrStopping, got %v", err)
	}
}

func TestTryStopDuringMigration(t *testing.T) {
	var g Guard
	end, err := g.Begin(Update)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Running() {
		t.Error("Running should be true during a migration")
	}
	if g.TryStop() {
		t.Fatal("TryStop should fail while a migration is running")
	}

	// a failed TryStop changes nothing: another migration can still start
	end2, err := g.Begin(Update)
	if err != nil {
		t.Fatalf("Begin after a failed TryStop: %v", err)
	}
	end2()
	end()
	end() // calling end twice counts once

	if g.Running() {
		t.Error("Running should be false once the migrations have ended")
	}
	if !g.TryStop() {
		t.Error("TryStop should succeed once the migrations have ended")
	}
}

func TestStopWaitsForMigration(t *testing.T) {
	var g Guard
	end, err := g.Begin(Update)
	if err != nil {
		t.Fatal(err)
	}

	waited := make(chan struct{})
	returned := make(chan struct{})
	go func() {
		g.Stop(func() { close(waited) })
		close(returned)
	}()

	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not say it was waiting")
	}
	select {
	case <-returned:
		t.Fatal("Stop returned while the migration was running")
	case <-time.After(50 * time.Millisecond):
	}

	// no new migration starts once Stop has been called
	if _, err := g.Begin(Update); !errors.Is(err, ErrStopping) {
		t.Errorf("Begin during Stop should return ErrStopping, got %v", err)
	}

	end()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return once the migration had finished")
	}
}

func TestStopWithoutMigration(t *testing.T) {
	var g Guard
	g.Stop(func() { t.Error("Stop should not say it is waiting when no migration is running") })
	g.Stop(nil) // a second Stop returns at once too
}

func TestDoing(t *testing.T) {
	var g Guard
	if k := g.Doing(); k != "" {
		t.Errorf("Doing with no migration = %q, want empty", k)
	}
	endUpdate, _ := g.Begin(Update)
	if k := g.Doing(); k != Update {
		t.Errorf("Doing during an update = %q, want %q", k, Update)
	}
	endBuild, _ := g.Begin(Build)
	if k := g.Doing(); k != Build {
		t.Errorf("Doing during a build and an update = %q, want %q", k, Build)
	}
	endBuild()
	if k := g.Doing(); k != Update {
		t.Errorf("Doing once the build ended = %q, want %q", k, Update)
	}
	endUpdate()
	if k := g.Doing(); k != "" {
		t.Errorf("Doing once all ended = %q, want empty", k)
	}
}
