//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoryStorageMigration(t *testing.T) {
	_ = config.InitializeEmpty()
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = migrationFixture(t)
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < 22; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	_, err = m.conn.Exec("INSERT INTO audios (id,title,created_at,updated_at) VALUES (77,'Existing audio',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)")
	require.NoError(t, err)
	require.NoError(t, m.RunMigration(ctx, 22))
	var title string
	require.NoError(t, m.conn.Get(&title, "SELECT title FROM audios WHERE id=77"))
	assert.Equal(t, "Existing audio", title)
	for _, table := range []string{"stories", "stories_files", "stories_tags", "story_urls", "stories_user_data"} {
		var count int
		require.NoError(t, m.conn.Get(&count, "SELECT COUNT(*) FROM "+table))
		assert.Zero(t, count)
	}
}
