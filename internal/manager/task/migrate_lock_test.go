package task

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/database"
)

// lockTestDatabase fails its upgrade, like guardTestDatabase, with a
// migrator that holds an upgrade lock, and notes whether the lock was still
// held while the backup was put back.
type lockTestDatabase struct {
	guardTestDatabase
	locked             bool
	lockedWhileRestore bool
}

type lockTestMigrator struct {
	guardTestMigrator
	db   *lockTestDatabase
	kept bool
}

func (d *lockTestDatabase) NewMigrator() (database.MigrateStore, error) {
	d.locked = true
	return &lockTestMigrator{guardTestMigrator: guardTestMigrator{db: &d.guardTestDatabase}, db: d}, nil
}

func (d *lockTestDatabase) RestoreFromBackup(path string) error {
	d.lockedWhileRestore = d.locked
	return d.guardTestDatabase.RestoreFromBackup(path)
}

func (m *lockTestMigrator) Close() {
	if !m.kept {
		m.db.locked = false
	}
}

func (m *lockTestMigrator) KeepUpgradeLock() func() {
	m.kept = true
	return func() { m.db.locked = false }
}

// The upgrade lock is kept until the backup has been put back after a failed
// upgrade, so that a second Stash waiting for it doesn't take the upgrade as
// done while the restore runs (review of #2232).
func TestMigrateJobKeepsLockUntilRestored(t *testing.T) {
	dir := t.TempDir()
	db := &lockTestDatabase{guardTestDatabase: guardTestDatabase{dir: dir}}
	j := MigrateJob{Config: guardTestConfig{dir: dir}, Database: db}

	if err := j.Execute(context.Background(), nil); err == nil {
		t.Fatal("the upgrade should fail, as the test migration does")
	}
	if !db.restored {
		t.Fatal("the failed upgrade should put the backup back")
	}
	if !db.lockedWhileRestore {
		t.Error("the upgrade lock was released before the backup was put back")
	}
	if db.locked {
		t.Error("the upgrade lock should be released once the job has finished")
	}
}
