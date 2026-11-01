//go:build integration

package postgres

import (
	"context"
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
			require.NoError(t, db.Open(ctx, migrationFixture(t)))
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
				if _, err := dbWrapper.Exec(ctx, "CREATE TEMP TABLE portrait_commit_failure (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES portrait_commit_failure(id) DEFERRABLE INITIALLY DEFERRED) ON COMMIT DROP"); err != nil {
					return err
				}
				_, err = dbWrapper.Exec(ctx, "INSERT INTO portrait_commit_failure VALUES (1, -1)")
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
