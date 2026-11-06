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

// #2196: migration 109 adds the child_id index on tags_relations and the
// tag_id index on tag_stash_ids, also on a database where someone already
// made one of them by hand.
func TestMigration109TagRelationIndexes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		before string
	}{
		{"neither index", ""},
		{"stash id index made by hand", "CREATE INDEX index_tag_stash_ids_on_tag_id ON tag_stash_ids (tag_id)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := NewDatabase()
			db.dbPath = filepath.Join(t.TempDir(), "stash-go.sqlite")
			m, err := NewMigrator(db)
			require.NoError(t, err)
			defer m.Close()
			for version := uint(1); version < 109; version++ {
				require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
			}
			if tt.before != "" {
				_, err = m.conn.Exec(tt.before)
				require.NoError(t, err)
			}

			require.NoError(t, m.RunMigration(ctx, 109))

			for index, want := range map[string]string{
				"index_tags_relations_on_child_id": "child_id,parent_id",
				"index_tag_stash_ids_on_tag_id":    "tag_id",
			} {
				var cols []string
				require.NoError(t, m.conn.Select(&cols, "SELECT name FROM pragma_index_info(?) ORDER BY seqno", index))
				assert.Equal(t, want, strings.Join(cols, ","), index)
			}
		})
	}
}
