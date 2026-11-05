//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/database/migrationguard"
	"github.com/stretchr/testify/require"
)

// watchMigrations records each kind of migration seen running until the
// returned function is called, which returns them.
func watchMigrations() func() map[migrationguard.Kind]bool {
	seen := map[migrationguard.Kind]bool{}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if k := migrationguard.Doing(); k != "" {
				seen[k] = true
			}
			select {
			case <-done:
				return
			case <-time.After(100 * time.Microsecond):
			}
		}
	}()
	return func() map[migrationguard.Kind]bool {
		close(done)
		wg.Wait()
		return seen
	}
}

// A stop waits for a migration only if it registers with migrationguard, so
// building a new database and updating an old one must both register (#2103).
func TestRunAllMigrationsRegistersWithGuard(t *testing.T) {
	ctx := context.Background()

	t.Run("build", func(t *testing.T) {
		stop := watchMigrations()
		db := NewDatabase()
		err := db.Open(ctx, filepath.Join(t.TempDir(), "new.sqlite"))
		seen := stop()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		require.True(t, seen[migrationguard.Build], "building a new database should register as a build, saw %v", seen)
	})

	t.Run("update", func(t *testing.T) {
		db, err := openAt(t, filepath.Join(t.TempDir(), "old.sqlite"), 50)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		stop := watchMigrations()
		err = db.RunAllMigrations(ctx)
		seen := stop()
		require.NoError(t, err)

		require.True(t, seen[migrationguard.Update], "updating a database should register as an update, saw %v", seen)
	})
}
