package task

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stashapp/stash/pkg/database"
)

// What the failed migration says about putting the backup back (#2117).
func TestRestoreMessage(t *testing.T) {
	const migrateErr = "error performing migration: forced"

	t.Run("restored", func(t *testing.T) {
		msg := restoreMessage(nil, migrateErr)
		assert.Contains(t, msg, "The database was automatically restored from the backup.")
		assert.Contains(t, msg, migrateErr)
	})

	t.Run("restored but not reopened", func(t *testing.T) {
		err := fmt.Errorf("%w (from x.sqlite): %w", database.ErrRestoredNotReopened, errors.New("disk on fire"))
		msg := restoreMessage(err, migrateErr)
		assert.Contains(t, msg, "the database was restored from the backup")
		assert.Contains(t, msg, "couldn't be opened again")
		assert.Contains(t, msg, "disk on fire")
		assert.Contains(t, msg, migrateErr)
		assert.NotContains(t, msg, "unable to restore")
	})

	t.Run("not restored", func(t *testing.T) {
		msg := restoreMessage(errors.New("access is denied"), migrateErr)
		assert.Contains(t, msg, "unable to restore database from backup")
		assert.Contains(t, msg, "access is denied")
		assert.Contains(t, msg, migrateErr)
	})
}
