package task

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/database"
)

// elsewhereTestMigrator waited while another Stash upgraded the database,
// and that one stopped part way.
type elsewhereTestMigrator struct{ guardTestMigrator }

func (m *elsewhereTestMigrator) RunMigration(context.Context, uint) error {
	return fmt.Errorf("%w: it is at schema version 100", database.ErrUpgradeUnfinishedElsewhere)
}

type elsewhereTestDatabase struct{ guardTestDatabase }

func (d *elsewhereTestDatabase) NewMigrator() (database.MigrateStore, error) {
	return &elsewhereTestMigrator{guardTestMigrator{db: &d.guardTestDatabase}}, nil
}

// When another Stash was upgrading the database at the same time, this one
// changed nothing, and its backup may be older than what the other did, so
// the backup isn't put back (#2213).
func TestMigrateJobDoesNotRestoreOverAnotherUpgrade(t *testing.T) {
	dir := t.TempDir()
	db := &elsewhereTestDatabase{guardTestDatabase{dir: dir}}
	j := MigrateJob{Config: guardTestConfig{dir: dir}, Database: db}

	err := j.Execute(context.Background(), nil)
	if !errors.Is(err, database.ErrUpgradeUnfinishedElsewhere) {
		t.Fatalf("the upgrade should say another Stash didn't finish it, got %v", err)
	}
	if db.restored {
		t.Error("the backup was put back over the other Stash's upgrade")
	}
}
