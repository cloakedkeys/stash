//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stashapp/stash/pkg/hash/md5"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformerPortraitRollback(t *testing.T) {
	var source, destination int
	var a, b string
	require.NoError(t, withTxn(func(ctx context.Context) error {
		source = createPortraitPerformer(t, ctx, t.Name()+" source")
		destination = createPortraitPerformer(t, ctx, t.Name()+" destination")
		require.NoError(t, db.Performer().UpdateImage(ctx, source, []byte("rollback source primary")))
		a = addPortrait(t, ctx, source, "rollback alternate a")
		b = addPortrait(t, ctx, source, "rollback alternate b")
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, withTxn(func(ctx context.Context) error {
			for _, id := range []int{source, destination} {
				if err := db.Performer().Destroy(ctx, id); err != nil {
					return err
				}
			}
			return nil
		}))
	})
	forced := errors.New("force portrait rollback")
	err := withTxn(func(ctx context.Context) error {
		require.NoError(t, db.Performer().ReorderAlternateImages(ctx, source, []string{b, a}))
		require.NoError(t, db.Performer().SetPrimaryImage(ctx, source, b))
		addPortrait(t, ctx, source, "rollback new portrait")
		require.NoError(t, db.Performer().Merge(ctx, []int{source}, destination))
		return forced
	})
	require.ErrorIs(t, err, forced)
	require.NoError(t, txn.WithReadTxn(context.Background(), db, func(ctx context.Context) error {
		assert.Equal(t, []string{a, b}, portraitChecksums(t, ctx, source))
		assert.Empty(t, portraitChecksums(t, ctx, destination))
		data, err := db.Performer().GetImage(ctx, source)
		require.NoError(t, err)
		assert.Equal(t, []byte("rollback source primary"), data)
		exists, err := db.Blobs().EntryExists(ctx, md5.FromBytes([]byte("rollback new portrait")))
		require.NoError(t, err)
		assert.False(t, exists)
		return nil
	}))
}

func TestPerformerUpdateImageKeepsPrimaryBlob(t *testing.T) {
	runWithRollbackTxn(t, "primary references", func(t *testing.T, ctx context.Context) {
		id := createPortraitPerformer(t, ctx, t.Name())
		qb := db.Performer()
		a := addPortrait(t, ctx, id, "primary and alternate a")
		b := addPortrait(t, ctx, id, "primary and alternate b")
		// Replacing the primary removes the matching alternate reference, not its blob.
		require.NoError(t, qb.UpdateImage(ctx, id, []byte("primary and alternate a")))
		require.Error(t, qb.SetPrimaryImage(ctx, id, a))
		assert.Equal(t, []string{b}, portraitChecksums(t, ctx, id))
		require.NoError(t, qb.UpdateImage(ctx, id, []byte("primary and alternate b")))
		assert.Empty(t, portraitChecksums(t, ctx, id))
		require.Error(t, qb.RemoveAlternateImage(ctx, id, b))
		data, err := qb.GetImage(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, []byte("primary and alternate b"), data)
		require.NoError(t, qb.Destroy(ctx, id))
		exists, err := db.Blobs().EntryExists(ctx, b)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}
