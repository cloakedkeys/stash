package manager

import (
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/database/migrationguard"
)

// closeRecorder is a database that only records when it is closed. Every
// other method is left unimplemented.
type closeRecorder struct {
	database.Database
	closed chan struct{}
}

func (d *closeRecorder) Close() error {
	close(d.closed)
	return nil
}

// Shutdown waits for a running database migration to finish before it closes
// the database, so the migration isn't cut off halfway (#2103).
func TestShutdownWaitsForMigration(t *testing.T) {
	var g migrationguard.Guard
	end, err := g.Begin(migrationguard.Update)
	if err != nil {
		t.Fatal(err)
	}

	db := &closeRecorder{closed: make(chan struct{})}
	m := &Manager{Database: db, migrations: &g}
	returned := make(chan struct{})
	go func() {
		m.Shutdown()
		close(returned)
	}()

	select {
	case <-db.closed:
		t.Fatal("Shutdown closed the database while a migration was running")
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := g.Begin(migrationguard.Update); err == nil {
		t.Error("no new migration should start once Shutdown has begun")
	}

	end()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not finish once the migration had")
	}
	select {
	case <-db.closed:
	default:
		t.Error("Shutdown should close the database once the migration has finished")
	}
}
