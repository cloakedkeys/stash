// Package migrationguard keeps a stop from cutting off a database migration
// halfway, which would leave the database part upgraded and marked as
// unfinished (#2103).
//
// Code that builds or upgrades a database schema calls Begin first and the
// returned end function once it has finished. Stopping Stash calls TryStop or
// Stop, which wait for running migrations to finish, and from then on Begin
// refuses to start new ones.
package migrationguard

import (
	"errors"
	"sync"
)

// Kind says what a migration does to the database, for messages.
type Kind string

const (
	// Build creates the schema of a new database.
	Build Kind = "build"
	// Update brings an existing database up to the current schema.
	Update Kind = "update"
)

// ErrStopping is returned by Begin once Stash has started to stop.
var ErrStopping = errors.New("stash is stopping, so the database migration was not started")

// Guard counts the migrations that are running. The zero value is ready to use.
type Guard struct {
	mu       sync.Mutex
	idle     *sync.Cond
	running  int
	kinds    map[Kind]int
	stopping bool
}

func (g *Guard) cond() *sync.Cond {
	if g.idle == nil {
		g.idle = sync.NewCond(&g.mu)
	}
	return g.idle
}

// Begin marks the start of a migration of the given kind. It returns the
// function to call when the migration has finished, or ErrStopping, in which
// case the migration must not start.
func (g *Guard) Begin(kind Kind) (end func(), err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.stopping {
		return nil, ErrStopping
	}
	g.running++
	if g.kinds == nil {
		g.kinds = make(map[Kind]int)
	}
	g.kinds[kind]++

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.running--
			g.kinds[kind]--
			if g.running == 0 {
				g.cond().Broadcast()
			}
		})
	}, nil
}

// Running reports whether a migration is running.
func (g *Guard) Running() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running > 0
}

// Doing returns the kind of migration running: Build if a database is being
// built, otherwise Update if one is being updated, otherwise "".
func (g *Guard) Doing() Kind {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case g.kinds[Build] > 0:
		return Build
	case g.kinds[Update] > 0:
		return Update
	}
	return ""
}

// TryStop stops new migrations from starting and returns true, if none is
// running. If one is running, it changes nothing and returns false.
func (g *Guard) TryStop() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running > 0 {
		return false
	}
	g.stopping = true
	return true
}

// Stop stops new migrations from starting, then waits until those running
// have finished. If any is running, it calls waiting first, unless waiting is
// nil.
func (g *Guard) Stop(waiting func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stopping = true
	if g.running == 0 {
		return
	}
	if waiting != nil {
		g.mu.Unlock()
		waiting()
		g.mu.Lock()
	}
	for g.running > 0 {
		g.cond().Wait()
	}
}

var std Guard

// Default returns the guard for this process.
func Default() *Guard { return &std }

// Begin marks the start of a migration on the guard for this process. See
// Guard.Begin.
func Begin(kind Kind) (end func(), err error) { return std.Begin(kind) }

// Running reports whether a migration is running in this process.
func Running() bool { return std.Running() }

// Doing calls Guard.Doing on the guard for this process.
func Doing() Kind { return std.Doing() }

// TryStop calls Guard.TryStop on the guard for this process.
func TryStop() bool { return std.TryStop() }

// Stop calls Guard.Stop on the guard for this process.
func Stop(waiting func()) { std.Stop(waiting) }
