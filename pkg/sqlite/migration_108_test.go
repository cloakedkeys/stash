//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tagJoinIndexes are the tables migration 108 changes, with their item column.
var tagJoinIndexes = []struct{ table, itemCol string }{
	{"scenes_tags", "scene_id"},
	{"images_tags", "image_id"},
	{"galleries_tags", "gallery_id"},
	{"performers_tags", "performer_id"},
	{"studios_tags", "studio_id"},
	{"groups_tags", "group_id"},
	{"audios_tags", "audio_id"},
	{"stories_tags", "story_id"},
	{"scene_markers_tags", "scene_marker_id"},
}

// #2192: migration 108 upgrades a database where some of the old tag_id
// indexes are missing, for example after a repair tool or a hand edit, and
// still makes all of the new ones.
func TestMigration108MissingOldIndexes(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "stash-go.sqlite")
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < 108; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}

	// one table without its index, and one with it
	_, err = m.conn.Exec("DROP INDEX index_scenes_tags_on_tag_id")
	require.NoError(t, err)

	require.NoError(t, m.RunMigration(ctx, 108))

	for _, tt := range tagJoinIndexes {
		index := "index_" + tt.table + "_on_tag_id"
		var cols []string
		require.NoError(t, m.conn.Select(&cols, "SELECT name FROM pragma_index_info(?) ORDER BY seqno", index))
		assert.Equal(t, "tag_id,"+tt.itemCol, strings.Join(cols, ","), index)
	}
}
