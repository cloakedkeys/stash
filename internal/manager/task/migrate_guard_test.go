package task

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/database/migrationguard"
)

type guardTestConfig struct{ dir string }

func (c guardTestConfig) GetBackupDirectoryPath() string          { return c.dir }
func (c guardTestConfig) GetBackupDirectoryPathOrDefault() string { return c.dir }

// guardTestDatabase is a database at version 100 whose migration to 101
// fails, so that the upgrade task stops before its last steps, which need
// the whole manager. Every other method is left unimplemented.
type guardTestDatabase struct {
	database.Database
	dir      string
	restored bool
	// restoredDuring is the migration registered while it rolled back
	restoredDuring migrationguard.Kind
	// seen is the migration registered while the upgrade ran, if any
	seen migrationguard.Kind
}

func (d *guardTestDatabase) NewMigrator() (database.MigrateStore, error) {
	return &guardTestMigrator{db: d}, nil
}
func (d *guardTestDatabase) DatabaseBackupPath(string) string { return filepath.Join(d.dir, "backup") }
func (d *guardTestDatabase) Backup(string) error              { return nil }
func (d *guardTestDatabase) RestoreFromBackup(string) error {
	d.restored = true
	d.restoredDuring = migrationguard.Doing()
	return nil
}

type guardTestMigrator struct{ db *guardTestDatabase }

func (m *guardTestMigrator) Close()                            {}
func (m *guardTestMigrator) CurrentSchemaVersion() uint        { return 100 }
func (m *guardTestMigrator) RequiredSchemaVersion() uint       { return 101 }
func (m *guardTestMigrator) PostMigrate(context.Context) error { return nil }
func (m *guardTestMigrator) RunMigration(context.Context, uint) error {
	m.db.seen = migrationguard.Doing()
	return errors.New("migration failed")
}

// A stop waits for an upgrade started from the web page only if the upgrade
// registers with migrationguard. It stays registered until it has rolled the
// database back (#2103).
func TestMigrateJobRegistersWithGuard(t *testing.T) {
	dir := t.TempDir()
	db := &guardTestDatabase{dir: dir}
	j := MigrateJob{Config: guardTestConfig{dir: dir}, Database: db}

	if err := j.Execute(context.Background(), nil); err == nil {
		t.Fatal("the upgrade should fail, as the test migration does")
	}
	if db.seen != migrationguard.Update {
		t.Errorf("the upgrade should be registered as an update while it runs, saw %q", db.seen)
	}
	if !db.restored {
		t.Error("the failed upgrade should roll the database back")
	}
	if db.restoredDuring != migrationguard.Update {
		t.Errorf("the upgrade should still be registered while it rolls back, saw %q", db.restoredDuring)
	}
	if migrationguard.Running() {
		t.Error("the upgrade should end its registration once it has finished")
	}
}
