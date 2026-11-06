package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func tagCountTestContext(t *testing.T) context.Context {
	t.Helper()
	db, err := sqlx.Open(sqlite3Driver, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	// The tables and indexes match the real schema.
	_, err = db.Exec(`CREATE TABLE tags (id INTEGER PRIMARY KEY);
CREATE TABLE tags_relations (parent_id INTEGER, child_id INTEGER, PRIMARY KEY(parent_id, child_id));
CREATE INDEX index_tags_relations_on_child_id ON tags_relations (child_id, parent_id);
CREATE TABLE scenes_tags (scene_id INTEGER, tag_id INTEGER, PRIMARY KEY(scene_id, tag_id));
CREATE INDEX index_scenes_tags_on_tag_id ON scenes_tags (tag_id);
CREATE TABLE scene_markers (id INTEGER PRIMARY KEY, primary_tag_id INTEGER);
CREATE INDEX index_scene_markers_on_primary_tag_id ON scene_markers (primary_tag_id);
CREATE TABLE scene_markers_tags (scene_marker_id INTEGER, tag_id INTEGER, PRIMARY KEY(scene_marker_id, tag_id));
CREATE INDEX index_scene_markers_tags_on_tag_id ON scene_markers_tags (tag_id);`)
	require.NoError(t, err)
	tx, err := db.Beginx()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	return context.WithValue(context.Background(), txnKey, tx)
}

func tagCountTestQuery(ctx context.Context, t *testing.T, filter *models.TagFilterType) queryBuilder {
	t.Helper()
	f := filterBuilderFromHandler(ctx, &tagFilterHandler{tagFilter: filter})
	require.NoError(t, f.getError())
	q := tagRepository.newQuery()
	q.columns = []string{"DISTINCT tags.id"}
	q.from = tagTable
	q.sortAndPagination = " ORDER BY tags.id"
	require.NoError(t, q.addFilter(f))
	return q
}

// #2180: counting the scenes of every tag with its sub-tags looked up the
// scenes of each tag once for each of its parents. With 5,000 tags and 50,000
// scenes that took seconds. Each scene and tag pair is now read once, from
// the table itself.
func TestTagHierarchicalCountPlan(t *testing.T) {
	ctx := tagCountTestContext(t)
	// 2,000 tags, each tag x >= 5 a child of x / 5; 20,000 scenes with 3 tags
	// each; 5,000 markers.
	_, err := dbWrapper.Exec(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
INSERT INTO scenes_tags SELECT DISTINCT x, 1 + (x * k) % 2000 FROM n, (SELECT 7 AS k UNION ALL SELECT 13 UNION ALL SELECT 17);
INSERT INTO tags SELECT DISTINCT tag_id FROM scenes_tags;
INSERT INTO tags_relations SELECT id / 5, id FROM tags WHERE id >= 5 AND id / 5 IN (SELECT id FROM tags);
INSERT INTO scene_markers SELECT scene_id, tag_id FROM scenes_tags WHERE scene_id <= 5000 GROUP BY scene_id;
INSERT INTO scene_markers_tags SELECT id, 1 + (id * 31) % 2000 FROM scene_markers;
ANALYZE;`)
	require.NoError(t, err)

	all := -1
	for _, tt := range []struct {
		name   string
		filter *models.TagFilterType
		table  string
	}{
		{"scenes", &models.TagFilterType{SceneCount: &models.HierarchicalCountInput{Modifier: models.CriterionModifierGreaterThan, Value: 10, Depth: &all}}, "scenes_tags"},
		{"markers", &models.TagFilterType{MarkerCount: &models.HierarchicalCountInput{Modifier: models.CriterionModifierGreaterThan, Value: 10, Depth: &all}}, "scene_markers_tags"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := tagCountTestQuery(ctx, t, tt.filter)
			var plan []struct {
				ID     int    `db:"id"`
				Parent int    `db:"parent"`
				Unused int    `db:"notused"`
				Detail string `db:"detail"`
			}
			require.NoError(t, dbWrapper.Select(ctx, &plan, "EXPLAIN QUERY PLAN "+q.toSQL(true), q.allArgs()...))
			var details []string
			for _, step := range plan {
				details = append(details, step.Detail)
			}
			joinedPlan := strings.Join(details, "\n")
			require.Contains(t, joinedPlan, "SCAN "+tt.table+"\n", "read the tag pairs once:\n%s", joinedPlan)
			require.NotContains(t, joinedPlan, "SEARCH "+tt.table+" USING", "don't look up the items of each sub-tag:\n%s", joinedPlan)
		})
	}
}

// The counts themselves, on a tree where C has two parents, so A reaches C
// twice, and scene 1 has two tags under A.
func TestTagHierarchicalCountResults(t *testing.T) {
	ctx := tagCountTestContext(t)
	// tags 1=A 2=B 3=C 4=D 5=E: A -> B -> C, A -> D -> C, E alone
	_, err := dbWrapper.Exec(ctx, `INSERT INTO tags VALUES (1),(2),(3),(4),(5);
INSERT INTO tags_relations VALUES (1,2),(2,3),(1,4),(4,3);
INSERT INTO scenes_tags VALUES (1,2),(1,3),(2,3),(3,4),(4,1);
INSERT INTO scene_markers VALUES (1,3),(2,1),(3,5),(4,3);
INSERT INTO scene_markers_tags VALUES (1,2),(3,4);`)
	require.NoError(t, err)

	all, one := -1, 1
	count := func(modifier models.CriterionModifier, value int, depth *int) *models.HierarchicalCountInput {
		return &models.HierarchicalCountInput{Modifier: modifier, Value: value, Depth: depth}
	}
	tests := []struct {
		name   string
		filter *models.TagFilterType
		want   []int
	}{
		// scenes with sub-tags: A 4, B 2, C 2, D 3, E 0
		{"scenes equals", &models.TagFilterType{SceneCount: count(models.CriterionModifierEquals, 2, &all)}, []int{2, 3}},
		{"scenes greater", &models.TagFilterType{SceneCount: count(models.CriterionModifierGreaterThan, 2, &all)}, []int{1, 4}},
		{"scenes none", &models.TagFilterType{SceneCount: count(models.CriterionModifierEquals, 0, &all)}, []int{5}},
		{"scenes less", &models.TagFilterType{SceneCount: count(models.CriterionModifierLessThan, 3, &all)}, []int{2, 3, 5}},
		// one level down: A 3, B 2, C 2, D 3, E 0
		{"scenes depth one", &models.TagFilterType{SceneCount: count(models.CriterionModifierEquals, 3, &one)}, []int{1, 4}},
		// markers with sub-tags, by primary or other tag: A 4, B 2, C 2, D 3, E 1
		{"markers equals", &models.TagFilterType{MarkerCount: count(models.CriterionModifierEquals, 2, &all)}, []int{2, 3}},
		{"markers greater", &models.TagFilterType{MarkerCount: count(models.CriterionModifierGreaterThan, 2, &all)}, []int{1, 4}},
		{"markers primary only", &models.TagFilterType{MarkerCount: count(models.CriterionModifierEquals, 1, &all)}, []int{5}},
		// one level down: A 3, B 2, C 2, D 3, E 1
		{"markers depth one", &models.TagFilterType{MarkerCount: count(models.CriterionModifierEquals, 3, &one)}, []int{1, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := tagCountTestQuery(ctx, t, tt.filter).findIDs(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.want, ids)
		})
	}
}
