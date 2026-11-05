//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database/migrationguard"
	"github.com/stretchr/testify/require"
)

// A stop waits for a migration only if it registers with migrationguard, so
// building a new PostgreSQL database must register (#2103).
func TestRunAllMigrationsRegistersWithGuard(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	require.NoError(t, err)
	name := fmt.Sprintf("migration_guard_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec("DROP DATABASE " + name)
		require.NoError(t, err)
		require.NoError(t, admin.Close())
	})
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name
	config.InitializeEmpty()

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

	db := NewDatabase()
	err = db.Open(ctx, u.String())
	close(done)
	wg.Wait()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.True(t, seen[migrationguard.Build], "building a new database should register as a build, saw %v", seen)
}
