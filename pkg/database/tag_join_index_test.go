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

// #2192: the tag index of each tag join table also holds the item id, so
// finding or counting the items of a tag doesn't read every table row.
func TestTagJoinIndexesHoldItemID(t *testing.T) {
	tables := []struct{ table, itemCol string }{
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

	require.NoError(t, withTxn(func(ctx context.Context) error {
		for _, tt := range tables {
			index := "index_" + tt.table + "_on_tag_id"
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
			require.NoError(t, err, tt.table)
			var cols []string
			for _, r := range rows {
				cols = append(cols, fmt.Sprint(r[0]))
			}
			assert.Equal(t, "tag_id,"+tt.itemCol, strings.Join(cols, ","), index)
		}
		return nil
	}))
}
