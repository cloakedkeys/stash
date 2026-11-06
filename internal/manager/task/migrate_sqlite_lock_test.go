package task

import (
	"testing"

	"github.com/stashapp/stash/pkg/postgres"
	"github.com/stashapp/stash/pkg/sqlite"
)

// The upgrade task keeps the upgrade lock of both databases' migrators until
// it has put a backup back, and doesn't put one back over another Stash's
// upgrade (TestMigrateJobKeepsLockUntilRestored,
// TestMigrateJobDoesNotRestoreOverAnotherUpgrade). It finds the lock through
// upgradeLockKeeper, so both migrators must have one (#2213).
func TestMigratorsKeepUpgradeLock(t *testing.T) {
	for name, m := range map[string]interface{}{
		"SQLite":     &sqlite.Migrator{},
		"PostgreSQL": &postgres.Migrator{},
	} {
		if _, ok := m.(upgradeLockKeeper); !ok {
			t.Errorf("the %s migrator has no upgrade lock for the upgrade task to keep", name)
		}
	}
}
