//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/hash/md5"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformerPortraitFailedCommitKeepsFiles(t *testing.T) {
	for _, operation := range []string{"remove", "destroy"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			db := NewDatabase()
			require.NoError(t, db.Open(ctx, filepath.Join(dir, "library.sqlite")))
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			db.SetBlobStoreOptions(database.BlobStoreOptions{UseFilesystem: true, Path: dir})
			p := models.Performer{Name: t.Name()}
			data := []byte("alternate portrait")
			checksum := md5.FromBytes(data)
			require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
				if err := db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}); err != nil {
					return err
				}
				if err := db.Performer().UpdateImage(ctx, p.ID, []byte("primary portrait")); err != nil {
					return err
				}
				_, err := db.Performer().AddAlternateImage(ctx, p.ID, data)
				return err
			}))
			err := txn.WithTxn(ctx, db, func(ctx context.Context) error {
				var err error
				if operation == "destroy" {
					err = db.Performer().Destroy(ctx, p.ID)
				} else {
					err = db.Performer().RemoveAlternateImage(ctx, p.ID, checksum)
				}
				if err != nil {
					return err
				}
				// This statement succeeds, but the database refuses COMMIT.
				if _, err := dbWrapper.Exec(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
					return err
				}
				_, err = dbWrapper.Exec(ctx, "INSERT INTO performer_images (performer_id, image_blob, position) VALUES (-1, 'missing', 0)")
				return err
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "saving changes")
			require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
				got, err := db.Performer().GetAlternateImage(ctx, p.ID, checksum)
				require.NoError(t, err)
				assert.Equal(t, data, got)
				got, err = db.Performer().GetImage(ctx, p.ID)
				require.NoError(t, err)
				assert.Equal(t, []byte("primary portrait"), got)
				return nil
			}))
		})
	}
}
