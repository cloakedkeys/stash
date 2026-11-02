//go:build integration

package sqlite

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestStoryStorageMigration(t *testing.T) {
	_ = config.InitializeEmpty()
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "story.sqlite")
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < 103; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	_, err = m.conn.Exec("INSERT INTO audios (id,title,created_at,updated_at) VALUES (77,'Existing audio',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)")
	require.NoError(t, err)
	require.NoError(t, m.RunMigration(ctx, 103))
	var title string
	require.NoError(t, m.conn.Get(&title, "SELECT title FROM audios WHERE id=77"))
	assert.Equal(t, "Existing audio", title)
	for _, table := range []string{"stories", "stories_files", "stories_tags", "story_urls", "stories_user_data"} {
		var count int
		require.NoError(t, m.conn.Get(&count, "SELECT COUNT(*) FROM "+table))
		assert.Zero(t, count)
	}
}

// Existing stories keep everything and start with no stored text, and stored text
// goes with its story.
func TestStoryTextMigration(t *testing.T) {
	_ = config.InitializeEmpty()
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "story-text.sqlite")
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < 104; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	_, err = m.conn.Exec("INSERT INTO stories (id,title,created_at,updated_at) VALUES (5,'Existing story',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)")
	require.NoError(t, err)
	require.NoError(t, m.RunMigration(ctx, 104))

	var title string
	require.NoError(t, m.conn.Get(&title, "SELECT title FROM stories WHERE id=5"))
	assert.Equal(t, "Existing story", title)
	var count int
	require.NoError(t, m.conn.Get(&count, "SELECT COUNT(*) FROM stories_text"))
	assert.Zero(t, count)

	// migrations run with foreign keys off, so these use a connection like Stash's own
	conn, err := sqlx.Connect(sqlite3Driver, "file:"+db.dbPath+"?_fk=true")
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Exec("INSERT INTO stories_text (story_id,text) VALUES (5,'Once upon a time')")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO stories_text (story_id,text) VALUES (6,'No such story')")
	assert.Error(t, err, "text needs a story")
	_, err = conn.Exec("DELETE FROM stories WHERE id=5")
	require.NoError(t, err)
	require.NoError(t, conn.Get(&count, "SELECT COUNT(*) FROM stories_text"))
	assert.Zero(t, count)
}
