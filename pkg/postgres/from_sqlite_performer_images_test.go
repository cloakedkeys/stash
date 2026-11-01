//go:build integration

package postgres

import (
	"bytes"
	"context"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/hash/md5"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateFromSQLitePerformerImages(t *testing.T) {
	dsn := migrationFixture(t)
	_ = config.InitializeEmpty()
	ctx := context.Background()
	path, raw := newSQLiteLibrary(t)
	primary := md5.FromBytes([]byte("primary portrait"))
	a := md5.FromBytes([]byte("alternate a"))
	b := md5.FromBytes([]byte("alternate b"))
	for checksum, data := range map[string]string{primary: "primary portrait", a: "alternate a", b: "alternate b"} {
		_, err := raw.Exec("INSERT INTO blobs (checksum, blob) VALUES (?, ?)", checksum, []byte(data))
		require.NoError(t, err)
	}
	_, err := raw.Exec("INSERT INTO performers (id, name, image_blob, created_at, updated_at) VALUES (7, 'portraits', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", primary)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO performer_images (performer_id, image_blob, position) VALUES (7, ?, 8), (7, ?, 2)", a, b)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, MigrateFromSQLite(ctx, path, dsn, &out))
	assert.Contains(t, out.String(), "Copied performer_images: 2 rows")
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetBlobStoreOptions(database.BlobStoreOptions{UseDatabase: true})
	require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
		images, err := db.Performer().GetAlternateImages(ctx, 7)
		require.NoError(t, err)
		assert.Equal(t, []models.PerformerImage{{Checksum: b, Position: 2}, {Checksum: a, Position: 8}}, images)
		data, err := db.Performer().GetImage(ctx, 7)
		require.NoError(t, err)
		assert.Equal(t, []byte("primary portrait"), data)
		for checksum, want := range map[string]string{a: "alternate a", b: "alternate b"} {
			data, err := db.Performer().GetAlternateImage(ctx, 7, checksum)
			require.NoError(t, err)
			assert.Equal(t, []byte(want), data)
		}
		return nil
	}))
}
