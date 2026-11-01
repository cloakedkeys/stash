//go:build integration

package database_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/hash/md5"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite/blob"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createPortraitPerformer(t *testing.T, ctx context.Context, name string) int {
	t.Helper()
	p := models.Performer{Name: name}
	require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
	return p.ID
}

func addPortrait(t *testing.T, ctx context.Context, id int, data string) string {
	t.Helper()
	added, err := db.Performer().AddAlternateImage(ctx, id, []byte(data))
	require.NoError(t, err)
	require.True(t, added)
	return md5.FromBytes([]byte(data))
}

func portraitChecksums(t *testing.T, ctx context.Context, id int) []string {
	t.Helper()
	images, err := db.Performer().GetAlternateImages(ctx, id)
	require.NoError(t, err)
	ret := make([]string, len(images))
	for i, image := range images {
		ret[i] = image.Checksum
		if i > 0 {
			assert.LessOrEqual(t, images[i-1].Position, image.Position)
		}
	}
	return ret
}

// This shared suite uses SQLite normally and PostgreSQL when PGSQL_TEST is set.
func TestPerformerAlternateImages(t *testing.T) {
	runWithRollbackTxn(t, "references and order", func(t *testing.T, ctx context.Context) {
		qb := db.Performer()
		id := createPortraitPerformer(t, ctx, t.Name())
		other := createPortraitPerformer(t, ctx, t.Name()+" other")
		assert.Empty(t, portraitChecksums(t, ctx, id))
		require.NoError(t, qb.UpdateImage(ctx, id, []byte("primary")))
		primary := md5.FromBytes([]byte("primary"))
		a := addPortrait(t, ctx, id, "alternate a")
		b := addPortrait(t, ctx, id, "alternate b")
		images, err := qb.GetAlternateImages(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, []models.PerformerImage{{Checksum: a, Position: 0}, {Checksum: b, Position: 1}}, images)
		count, err := db.Blobs().Count(ctx)
		require.NoError(t, err)
		for _, data := range []string{"primary", "alternate a"} {
			added, err := qb.AddAlternateImage(ctx, id, []byte(data))
			require.NoError(t, err)
			assert.False(t, added)
		}
		after, err := db.Blobs().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, count, after)
		assert.Equal(t, []string{a, b}, portraitChecksums(t, ctx, id))
		data, err := qb.GetAlternateImage(ctx, id, a)
		require.NoError(t, err)
		assert.Equal(t, []byte("alternate a"), data)
		_, err = qb.GetAlternateImage(ctx, other, a)
		require.Error(t, err)
		require.Error(t, qb.SetPrimaryImage(ctx, other, a))
		require.Error(t, qb.RemoveAlternateImage(ctx, other, a))
		assert.Equal(t, []string{a, b}, portraitChecksums(t, ctx, id))

		// Invalid reorder requests must not partially change the order.
		for _, checksums := range [][]string{{a}, {a, a}, {a, "unknown"}} {
			require.Error(t, qb.ReorderAlternateImages(ctx, id, checksums))
			assert.Equal(t, []string{a, b}, portraitChecksums(t, ctx, id))
		}
		require.NoError(t, qb.ReorderAlternateImages(ctx, id, []string{b, a}))
		assert.Equal(t, []string{b, a}, portraitChecksums(t, ctx, id))
		require.NoError(t, qb.SetPrimaryImage(ctx, id, b))
		data, err = qb.GetImage(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, []byte("alternate b"), data)
		assert.Equal(t, []string{primary, a}, portraitChecksums(t, ctx, id))
		after, err = db.Blobs().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, count, after, "swapping only changes references")

		// The old primary writer still replaces only the primary.
		require.NoError(t, qb.UpdateImage(ctx, id, []byte("replacement")))
		assert.Equal(t, []string{primary, a}, portraitChecksums(t, ctx, id))
		require.NoError(t, qb.UpdateImage(ctx, id, nil))
		assert.Equal(t, []string{primary, a}, portraitChecksums(t, ctx, id))
		require.NoError(t, qb.SetPrimaryImage(ctx, id, a))
		data, err = qb.GetImage(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, []byte("alternate a"), data)
		assert.Equal(t, []string{primary}, portraitChecksums(t, ctx, id))
		require.NoError(t, qb.RemoveAlternateImage(ctx, id, primary))
		require.Error(t, qb.RemoveAlternateImage(ctx, id, primary), "only an owned alternate can be removed")
		assert.Empty(t, portraitChecksums(t, ctx, id))
		exists, err := db.Blobs().EntryExists(ctx, primary)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestPerformerUpdateImageRemovesMatchingAlternate(t *testing.T) {
	for _, empty := range [][]byte{nil, {}} {
		name := "nil"
		if empty != nil {
			name = "empty bytes"
		}
		runWithRollbackTxn(t, name, func(t *testing.T, ctx context.Context) {
			qb := db.Performer()
			id := createPortraitPerformer(t, ctx, t.Name())
			require.NoError(t, qb.UpdateImage(ctx, id, []byte("old primary")))
			chosen := addPortrait(t, ctx, id, "new primary")
			remaining := addPortrait(t, ctx, id, "remaining alternate")

			require.NoError(t, qb.UpdateImage(ctx, id, []byte("new primary")))
			assert.Equal(t, []string{remaining}, portraitChecksums(t, ctx, id))
			data, err := qb.GetImage(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, []byte("new primary"), data)
			exists, err := db.Blobs().EntryExists(ctx, chosen)
			require.NoError(t, err)
			assert.True(t, exists)
			exists, err = db.Blobs().EntryExists(ctx, md5.FromBytes([]byte("old primary")))
			require.NoError(t, err)
			assert.False(t, exists, "replacement still removes the old primary")

			require.NoError(t, qb.UpdateImage(ctx, id, empty))
			data, err = qb.GetImage(ctx, id)
			require.NoError(t, err)
			assert.Empty(t, data)
			assert.Equal(t, []string{remaining}, portraitChecksums(t, ctx, id))
			exists, err = db.Blobs().EntryExists(ctx, chosen)
			require.NoError(t, err)
			assert.False(t, exists, "clearing removes the primary blob")
			data, err = qb.GetAlternateImage(ctx, id, remaining)
			require.NoError(t, err)
			assert.Equal(t, []byte("remaining alternate"), data)
		})
	}
}

func TestPerformerAlternateImageSharing(t *testing.T) {
	runWithRollbackTxn(t, "shared blobs", func(t *testing.T, ctx context.Context) {
		qb := db.Performer()
		a := createPortraitPerformer(t, ctx, t.Name()+" a")
		b := createPortraitPerformer(t, ctx, t.Name()+" b")
		checksum := addPortrait(t, ctx, a, "shared portrait")
		addPortrait(t, ctx, b, "shared portrait")
		require.NoError(t, qb.RemoveAlternateImage(ctx, a, checksum))
		data, err := qb.GetAlternateImage(ctx, b, checksum)
		require.NoError(t, err)
		assert.Equal(t, []byte("shared portrait"), data)
		// Deleting another object's primary must also count alternate references.
		tag := models.Tag{Name: t.Name()}
		require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tag}))
		require.NoError(t, db.Tag().UpdateImage(ctx, tag.ID, data))
		require.NoError(t, db.Tag().UpdateImage(ctx, tag.ID, nil))
		data, err = qb.GetAlternateImage(ctx, b, checksum)
		require.NoError(t, err)
		assert.Equal(t, []byte("shared portrait"), data)

		// Alternates must not delete a different object's primary, either.
		require.NoError(t, db.Tag().UpdateImage(ctx, tag.ID, data))
		require.NoError(t, qb.UpdateImage(ctx, a, data))
		require.NoError(t, qb.Destroy(ctx, b))
		assert.Empty(t, portraitChecksums(t, ctx, b))
		data, err = qb.GetImage(ctx, a)
		require.NoError(t, err)
		assert.Equal(t, []byte("shared portrait"), data)
		require.NoError(t, qb.Destroy(ctx, a))
		data, err = db.Tag().GetImage(ctx, tag.ID)
		require.NoError(t, err)
		assert.Equal(t, []byte("shared portrait"), data)
		require.NoError(t, db.Tag().Destroy(ctx, tag.ID))
		exists, err := db.Blobs().EntryExists(ctx, checksum)
		require.NoError(t, err)
		assert.False(t, exists)

		id := createPortraitPerformer(t, ctx, t.Name()+" destroy")
		checksum = addPortrait(t, ctx, id, "unshared portrait")
		require.NoError(t, qb.Destroy(ctx, id))
		assert.Empty(t, portraitChecksums(t, ctx, id))
		exists, err = db.Blobs().EntryExists(ctx, checksum)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestPerformerMergePortraits(t *testing.T) {
	for _, hasPrimary := range []bool{false, true} {
		runWithRollbackTxn(t, map[bool]string{false: "no destination primary", true: "destination primary"}[hasPrimary], func(t *testing.T, ctx context.Context) {
			qb := db.Performer()
			destination := createPortraitPerformer(t, ctx, t.Name()+" destination")
			onlyAlternates := createPortraitPerformer(t, ctx, t.Name()+" only alternates")
			source := createPortraitPerformer(t, ctx, t.Name()+" source")
			second := createPortraitPerformer(t, ctx, t.Name()+" second")
			third := createPortraitPerformer(t, ctx, t.Name()+" third")
			if hasPrimary {
				require.NoError(t, qb.UpdateImage(ctx, destination, []byte("destination primary")))
			}
			a := addPortrait(t, ctx, destination, "destination alternate")
			require.NoError(t, qb.UpdateImage(ctx, source, []byte("source primary")))
			b := md5.FromBytes([]byte("source primary"))
			addPortrait(t, ctx, onlyAlternates, "source primary")
			addPortrait(t, ctx, onlyAlternates, "destination alternate")
			addPortrait(t, ctx, source, "destination alternate")
			c := addPortrait(t, ctx, source, "source alternate")
			if hasPrimary {
				addPortrait(t, ctx, source, "destination primary")
			}
			require.NoError(t, qb.UpdateImage(ctx, second, []byte("source primary")))
			addPortrait(t, ctx, second, "source alternate")
			require.NoError(t, qb.UpdateImage(ctx, third, []byte("later primary")))
			d := md5.FromBytes([]byte("later primary"))
			require.NoError(t, qb.Merge(ctx, []int{onlyAlternates, source, second, third}, destination))
			if hasPrimary {
				assert.Equal(t, []string{a, b, c, d}, portraitChecksums(t, ctx, destination))
			} else {
				assert.Equal(t, []string{a, c, d}, portraitChecksums(t, ctx, destination))
			}
			for _, id := range []int{onlyAlternates, source, second, third} {
				p, err := qb.Find(ctx, id)
				require.NoError(t, err)
				assert.Nil(t, p)
				assert.Empty(t, portraitChecksums(t, ctx, id))
			}
			alternates := map[string]string{a: "destination alternate", c: "source alternate", d: "later primary"}
			if hasPrimary {
				alternates[b] = "source primary"
			}
			for checksum, want := range alternates {
				data, err := qb.GetAlternateImage(ctx, destination, checksum)
				require.NoError(t, err)
				assert.Equal(t, []byte(want), data)
			}
			data, err := qb.GetImage(ctx, destination)
			require.NoError(t, err)
			if hasPrimary {
				assert.Equal(t, []byte("destination primary"), data)
			} else {
				assert.Equal(t, []byte("source primary"), data)
			}
		})
	}
	runWithRollbackTxn(t, "sources only have alternates", func(t *testing.T, ctx context.Context) {
		qb := db.Performer()
		destination := createPortraitPerformer(t, ctx, t.Name()+" destination")
		source := createPortraitPerformer(t, ctx, t.Name()+" source")
		a := addPortrait(t, ctx, source, "source alternate")
		require.NoError(t, qb.Merge(ctx, []int{source}, destination))
		assert.Equal(t, []string{a}, portraitChecksums(t, ctx, destination))
		data, err := qb.GetImage(ctx, destination)
		require.NoError(t, err)
		assert.Empty(t, data)
		data, err = qb.GetAlternateImage(ctx, destination, a)
		require.NoError(t, err)
		assert.Equal(t, []byte("source alternate"), data)
	})
}

func TestPerformerPortraitFilesystemRollback(t *testing.T) {
	path := t.TempDir()
	db.SetBlobStoreOptions(database.BlobStoreOptions{UseFilesystem: true, Path: path})
	var id int
	checksum := md5.FromBytes([]byte(t.Name()))
	t.Cleanup(func() {
		defer db.SetBlobStoreOptions(database.BlobStoreOptions{UseDatabase: true})
		if id != 0 {
			require.NoError(t, withTxn(func(ctx context.Context) error { return db.Performer().Destroy(ctx, id) }))
		}
	})
	require.NoError(t, withTxn(func(ctx context.Context) error {
		id = createPortraitPerformer(t, ctx, t.Name())
		addPortrait(t, ctx, id, t.Name())
		return nil
	}))
	forced := errors.New("forced failure after deleting portrait")
	for _, operation := range []string{"remove", "destroy", "swap", "migrate"} {
		for _, failure := range []string{"callback", "precommit"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				if operation == "migrate" {
					db.SetBlobStoreOptions(database.BlobStoreOptions{UseDatabase: true, Path: path})
				}
				err := withTxn(func(ctx context.Context) error {
					var err error
					switch operation {
					case "remove":
						err = db.Performer().RemoveAlternateImage(ctx, id, checksum)
					case "destroy":
						err = db.Performer().Destroy(ctx, id)
					case "swap":
						err = db.Performer().SetPrimaryImage(ctx, id, checksum)
					case "migrate":
						err = db.Blobs().MigrateBlob(ctx, checksum, true)
					}
					require.NoError(t, err)
					if failure == "precommit" {
						txn.AddPreCommitHook(ctx, func(context.Context) error { return forced })
						return nil
					}
					return forced
				})
				require.ErrorIs(t, err, forced)
				db.SetBlobStoreOptions(database.BlobStoreOptions{UseFilesystem: true, Path: path})
				require.NoError(t, txn.WithReadTxn(context.Background(), db, func(ctx context.Context) error {
					data, err := db.Performer().GetAlternateImage(ctx, id, checksum)
					require.NoError(t, err)
					assert.Equal(t, []byte("TestPerformerPortraitFilesystemRollback"), data)
					return nil
				}))
			})
		}
	}
	reader := blob.NewReadonlyFilesystemStore(path, &file.OsFS{})
	data, err := reader.Read(context.Background(), checksum)
	require.NoError(t, err)
	assert.Equal(t, []byte(t.Name()), data)
	// The same cleanup path really deletes the blob when the transaction commits.
	require.NoError(t, withTxn(func(ctx context.Context) error {
		return db.Performer().RemoveAlternateImage(ctx, id, checksum)
	}))
	require.NoError(t, txn.WithReadTxn(context.Background(), db, func(ctx context.Context) error {
		exists, err := db.Blobs().EntryExists(ctx, checksum)
		require.NoError(t, err)
		assert.False(t, exists)
		return nil
	}))
	_, err = reader.Read(context.Background(), checksum)
	assert.Error(t, err, "the committed removal deletes the filesystem blob")
}

func TestAnonymiserClearsAlternatePortraits(t *testing.T) {
	var id int
	require.NoError(t, withTxn(func(ctx context.Context) error {
		id = createPortraitPerformer(t, ctx, t.Name())
		addPortrait(t, ctx, id, "private alternate portrait")
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, withTxn(func(ctx context.Context) error { return db.Performer().Destroy(ctx, id) }))
	})
	out := filepath.Join(t.TempDir(), "anonymous.sqlite")
	require.NoError(t, db.Anonymise(out))
	assert.Zero(t, countRows(t, out, "performer_images"))
	assert.Zero(t, countRows(t, out, "blobs"))
	require.NoError(t, txn.WithReadTxn(context.Background(), db, func(ctx context.Context) error {
		data, err := db.Performer().GetAlternateImage(ctx, id, md5.FromBytes([]byte("private alternate portrait")))
		require.NoError(t, err)
		assert.Equal(t, []byte("private alternate portrait"), data)
		return nil
	}))
}

func TestPerformerAlternateBlobStorageMigration(t *testing.T) {
	path := t.TempDir()
	var id int
	checksum := md5.FromBytes([]byte(t.Name()))
	t.Cleanup(func() {
		defer db.SetBlobStoreOptions(database.BlobStoreOptions{UseDatabase: true})
		if id != 0 {
			require.NoError(t, withTxn(func(ctx context.Context) error { return db.Performer().Destroy(ctx, id) }))
		}
	})
	require.NoError(t, withTxn(func(ctx context.Context) error {
		id = createPortraitPerformer(t, ctx, t.Name())
		addPortrait(t, ctx, id, t.Name())
		checksums, err := db.Blobs().FindBlobs(ctx, 1000, "")
		require.NoError(t, err)
		assert.Contains(t, checksums, checksum, "the migration enumerator includes alternate portraits")
		return nil
	}))
	for _, filesystem := range []bool{true, false} {
		db.SetBlobStoreOptions(database.BlobStoreOptions{UseFilesystem: filesystem, UseDatabase: !filesystem, Path: path})
		require.NoError(t, withTxn(func(ctx context.Context) error {
			return db.Blobs().MigrateBlob(ctx, checksum, true)
		}))
		require.NoError(t, txn.WithReadTxn(context.Background(), db, func(ctx context.Context) error {
			data, err := db.Performer().GetAlternateImage(ctx, id, checksum)
			require.NoError(t, err)
			assert.Equal(t, []byte(t.Name()), data)
			exists, err := db.Blobs().EntryExists(ctx, checksum)
			require.NoError(t, err)
			assert.True(t, exists, "the filesystem cleaner must retain this blob")
			return nil
		}))
	}
	reader := blob.NewReadonlyFilesystemStore(path, &file.OsFS{})
	_, err := reader.Read(context.Background(), checksum)
	assert.Error(t, err, "migration to the database removes the old file after commit")
}
