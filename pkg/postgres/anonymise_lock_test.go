//go:build integration

package postgres

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An anonymised export builds a new SQLite file of its own, so it leaves no
// upgrade lock file in the folder, whether it works or fails, and a failed
// one leaves no file at all (review of #2247).
func TestAnonymiseLeavesNoUpgradeLock(t *testing.T) {
	if os.Getenv("PGSQL_TEST") == "" {
		t.Skip("PGSQL_TEST is required")
	}

	lockFiles := func(dir string) []string {
		files, err := filepath.Glob(filepath.Join(dir, "*.upgrade-lock"))
		require.NoError(t, err)
		return files
	}

	t.Run("works", func(t *testing.T) {
		db := openTestDatabase(t)
		dir := t.TempDir()
		require.NoError(t, db.Anonymise(filepath.Join(dir, "anonymous.sqlite")))
		assert.FileExists(t, filepath.Join(dir, "anonymous.sqlite"))
		assert.Empty(t, lockFiles(dir))
	})

	t.Run("fails", func(t *testing.T) {
		db := openTestDatabase(t)
		// reading the database for the export fails
		require.NoError(t, db.Close())
		dir := t.TempDir()
		require.Error(t, db.Anonymise(filepath.Join(dir, "anonymous.sqlite")))
		assert.Empty(t, lockFiles(dir))
		// and no half-made export, as when anonymising fails later
		assert.NoFileExists(t, filepath.Join(dir, "anonymous.sqlite"))
	})
}
