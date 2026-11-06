//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2196: migration 29 adds the child_id index on tags_relations, also on a
// database where someone already made it by hand.
func TestMigration29TagRelationIndex(t *testing.T) {
	for _, tt := range []struct {
		name   string
		before string
	}{
		{"no index", ""},
		{"index made by hand", "CREATE INDEX index_tags_relations_on_child_id ON tags_relations (child_id, parent_id)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := NewDatabase()
			db.dbPath = migrationFixture(t)
			config.InitializeEmpty()
			m, err := NewMigrator(db)
			require.NoError(t, err)
			defer m.Close()
			for version := uint(1); version < 29; version++ {
				require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
			}
			if tt.before != "" {
				_, err = m.conn.Exec(tt.before)
				require.NoError(t, err)
			}

			require.NoError(t, m.RunMigration(ctx, 29))

			var cols []string
			require.NoError(t, m.conn.Select(&cols, `SELECT a.attname::text FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, n) ON true
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
WHERE c.relname = 'index_tags_relations_on_child_id' AND c.relnamespace = current_schema()::regnamespace
ORDER BY k.n`))
			assert.Equal(t, "child_id,parent_id", strings.Join(cols, ","))
		})
	}
}
