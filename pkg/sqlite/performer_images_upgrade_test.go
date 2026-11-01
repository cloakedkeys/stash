//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformerImagesUpgradePreservesPrimary(t *testing.T) {
	_ = config.InitializeEmpty()
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "library.sqlite")
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < appSchemaVersion; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	_, err = m.conn.Exec("INSERT INTO blobs (checksum, blob) VALUES ('portrait', ?)", []byte("existing portrait"))
	require.NoError(t, err)
	_, err = m.conn.Exec("INSERT INTO performers (id, name, image_blob, created_at, updated_at) VALUES (7, 'existing', 'portrait', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)")
	require.NoError(t, err)
	var before, after performerRow
	require.NoError(t, m.conn.Get(&before, "SELECT * FROM performers WHERE id = 7"))
	require.NoError(t, m.RunMigration(ctx, appSchemaVersion))
	require.NoError(t, m.conn.Get(&after, "SELECT * FROM performers WHERE id = 7"))
	assert.Equal(t, before, after)
	var data []byte
	require.NoError(t, m.conn.Get(&data, "SELECT blob FROM blobs WHERE checksum = 'portrait'"))
	assert.Equal(t, []byte("existing portrait"), data)
	var count int
	require.NoError(t, m.conn.Get(&count, "SELECT COUNT(*) FROM performer_images"))
	assert.Zero(t, count, "existing portraits stay primary without moving data")
}
