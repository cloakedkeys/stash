//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upgradeAsJob upgrades the database the way the upgrade task does: one
// version at a time, from the version it reads, until the build's version.
func upgradeAsJob(ctx context.Context, m *Migrator) error {
	for m.CurrentSchemaVersion() < m.RequiredSchemaVersion() {
		if err := m.RunMigration(ctx, m.CurrentSchemaVersion()+1); err != nil {
			return err
		}
	}
	return nil
}

// startSecondUpgrade starts a second Stash's upgrade of dsn, which has read
// the database's version already, and checks that it waits for the first.
func startSecondUpgrade(t *testing.T, ctx context.Context, dsn string) (*Migrator, chan error) {
	t.Helper()
	second := NewDatabase()
	second.dbPath = dsn
	m, err := NewMigrator(second)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	done := make(chan error, 1)
	go func() { done <- upgradeAsJob(ctx, m) }()
	select {
	case err := <-done:
		t.Fatalf("the second Stash didn't wait for the first to finish: %v", err)
	case <-time.After(time.Second):
	}
	return m, done
}

func waitFor(t *testing.T, done chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the second Stash didn't carry on once the first had finished")
		return nil
	}
}

// A second Stash that starts upgrading the database while another is doing
// it waits until the other has finished, then carries on with the upgraded
// database, rather than stopping with "invalid migration version" (#2213).
func TestSecondUpgradeWaitsForFirst(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	first := NewDatabase()
	first.dbPath = dsn
	m, err := NewMigrator(first)
	require.NoError(t, err)
	defer m.Close()
	require.NoError(t, m.RunMigration(ctx, 1))

	second, done := startSecondUpgrade(t, ctx, dsn)

	require.NoError(t, upgradeAsJob(ctx, m))
	m.Close()

	require.NoError(t, waitFor(t, done))
	assert.Equal(t, appSchemaVersion, second.CurrentSchemaVersion())
	version, dirty := readMigrationVersion(t, dsn)
	assert.Equal(t, int(appSchemaVersion), version)
	assert.False(t, dirty)
}

// If the first Stash stops part way through the upgrade, the second, which
// waited for it, doesn't take over: it runs no migration itself and says why,
// with an error the upgrade task knows not to put a backup back for (#2213).
func TestSecondUpgradeStopsWhenFirstStopsPartWay(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	first := NewDatabase()
	first.dbPath = dsn
	m, err := NewMigrator(first)
	require.NoError(t, err)
	defer m.Close()
	require.NoError(t, m.RunMigration(ctx, 1))

	_, done := startSecondUpgrade(t, ctx, dsn)

	const stoppedAt = 5
	for v := uint(2); v <= stoppedAt; v++ {
		require.NoError(t, m.RunMigration(ctx, v))
	}
	m.Close()

	err = waitFor(t, done)
	require.ErrorIs(t, err, database.ErrUpgradeUnfinishedElsewhere)
	assert.Contains(t, err.Error(), "Start Stash again")
	version, dirty := readMigrationVersion(t, dsn)
	assert.Equal(t, stoppedAt, version, "the second Stash ran a migration")
	assert.False(t, dirty)
}
