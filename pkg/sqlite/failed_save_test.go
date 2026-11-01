//go:build integration

package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The database refuses this save at the very last moment, so it's the real thing
// and not a stand-in: the caller is told, and the file marked for deletion comes back.
func TestFailedSaveKeepsFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	db := NewDatabase()
	require.NoError(t, db.Open(ctx, filepath.Join(dir, "stash.sqlite")))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	path := filepath.Join(dir, "video.mp4")
	require.NoError(t, os.WriteFile(path, []byte("video"), 0644))

	deleter := file.NewDeleter()
	err := txn.WithTxn(ctx, db, func(ctx context.Context) error {
		deleter.RegisterHooks(ctx)
		if err := deleter.Files([]string{path}); err != nil {
			return err
		}

		// a row that refers to nothing, which is only checked when saving
		if _, err := dbWrapper.Exec(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
			return err
		}
		_, err := dbWrapper.Exec(ctx, "INSERT INTO performers_scenes (performer_id, scene_id) VALUES (999999, 999999)")
		return err
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "FOREIGN KEY")
	assert.FileExists(t, path)
	assert.NoFileExists(t, path+".delete")

	// the database is still usable afterwards, and the row didn't stay
	var count int
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		return dbWrapper.Get(ctx, &count, "SELECT COUNT(*) FROM performers_scenes")
	}))
	assert.Zero(t, count)
}
