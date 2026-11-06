//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2196: finding a tag's parents and deleting a tag look rows up by child_id
// in tags_relations and by tag_id in tag_stash_ids, so both need an index that
// starts with that column.
func TestTagRelationIndexes(t *testing.T) {
	want := map[string]string{
		"index_tags_relations_on_child_id": "child_id,parent_id",
		"index_tag_stash_ids_on_tag_id":    "tag_id",
	}
	if IsPostgresTest() != nil {
		// PostgreSQL already has a unique index on tag_stash_ids that starts
		// with tag_id (migration 12), so it gets no new one.
		delete(want, "index_tag_stash_ids_on_tag_id")
		want["tag_stash_ids_unique_idx"] = "tag_id,endpoint,stash_id"
	}

	require.NoError(t, withTxn(func(ctx context.Context) error {
		for index, cols := range want {
			var q string
			if IsPostgresTest() != nil {
				q = fmt.Sprintf(`SELECT a.attname::text FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, n) ON true
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
WHERE c.relname = '%s' ORDER BY k.n`, index)
			} else {
				q = fmt.Sprintf(`SELECT name FROM pragma_index_info('%s') ORDER BY seqno`, index)
			}
			_, rows, err := db.QuerySQL(ctx, q, nil)
			require.NoError(t, err, index)
			var got []string
			for _, r := range rows {
				got = append(got, fmt.Sprint(r[0]))
			}
			assert.Equal(t, cols, strings.Join(got, ","), index)
		}
		return nil
	}))
}
