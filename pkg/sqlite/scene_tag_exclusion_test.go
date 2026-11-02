package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func sceneTagExclusionTestContext(t *testing.T) context.Context {
	t.Helper()
	db, err := sqlx.Open(sqlite3Driver, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`CREATE TABLE scenes (id INTEGER PRIMARY KEY);
CREATE TABLE tags (id INTEGER PRIMARY KEY);
CREATE TABLE tags_relations (parent_id INTEGER, child_id INTEGER);
CREATE TABLE scenes_tags (scene_id INTEGER, tag_id INTEGER, PRIMARY KEY(scene_id, tag_id));
CREATE INDEX index_scenes_tags_on_tag_id ON scenes_tags (tag_id);`)
	require.NoError(t, err)
	tx, err := db.Beginx()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	return context.WithValue(context.Background(), txnKey, tx)
}

func sceneTagExclusionTestQuery(ctx context.Context, t *testing.T, c models.HierarchicalMultiCriterionInput) queryBuilder {
	t.Helper()
	f := &filterBuilder{}
	(&sceneFilterHandler{}).tagsCriterionHandler(&c)(ctx, f)
	q := sceneRepository.newQuery()
	q.columns = []string{"DISTINCT scenes.id"}
	q.from = sceneTable
	q.sortAndPagination = " ORDER BY scenes.id"
	require.NoError(t, q.addFilter(f))
	return q
}

func TestSceneTagExclusionUsesSceneIndex(t *testing.T) {
	ctx := sceneTagExclusionTestContext(t)
	// One tag belongs to half the library; most tags belong to just one scene.
	// The average tag size underestimates the cost of materializing the big tag.
	_, err := dbWrapper.Exec(ctx, `WITH RECURSIVE n(x) AS (
VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000
) INSERT INTO scenes SELECT x FROM n;
INSERT INTO scenes_tags SELECT id, 1 FROM scenes WHERE id<=1000;
INSERT INTO scenes_tags SELECT id, id+1 FROM scenes;
ANALYZE;`)
	require.NoError(t, err)
	q := sceneTagExclusionTestQuery(ctx, t, models.HierarchicalMultiCriterionInput{
		Modifier: models.CriterionModifierExcludes,
		Value:    []string{"1"},
	})
	q.sortAndPagination += " LIMIT 50"

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
	require.NotContains(t, joinedPlan, "MATERIALIZE", "excluded associations must not be materialized: %s", joinedPlan)
	require.Contains(t, joinedPlan, "scene_id=?", "look up associations for the current scene: %s", joinedPlan)

	ids, err := q.findIDs(ctx)
	require.NoError(t, err)
	require.Len(t, ids, 50)
	require.Equal(t, 1001, ids[0])
	require.Equal(t, 1050, ids[49])
}

func TestSceneTagExclusionCombinations(t *testing.T) {
	ctx := sceneTagExclusionTestContext(t)
	_, err := dbWrapper.Exec(ctx, `INSERT INTO scenes VALUES (1),(2),(3),(4),(5),(6);
INSERT INTO tags VALUES (1),(2),(3),(4);
INSERT INTO tags_relations VALUES (1,2),(2,3);
INSERT INTO scenes_tags VALUES (1,1),(1,4),(2,2),(2,4),(3,3),(3,4),(4,4),(6,2),(6,3);`)
	require.NoError(t, err)
	depthOne, depthAll := 1, -1
	tests := []struct {
		name string
		c    models.HierarchicalMultiCriterionInput
		want []int
	}{
		{"excludes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierExcludes, Value: []string{"1"}}, []int{2, 3, 4, 5, 6}},
		{"multiple excludes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierExcludes, Value: []string{"1", "2"}}, []int{3, 4, 5}},
		{"both exclusion fields", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierExcludes, Value: []string{"1"}, Excludes: []string{"2"}}, []int{3, 4, 5}},
		{"includes and excludes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierIncludes, Value: []string{"4"}, Excludes: []string{"1", "2"}}, []int{3, 4}},
		{"includes all and excludes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierIncludesAll, Value: []string{"2", "3"}, Excludes: []string{"1"}}, []int{6}},
		{"equals and excludes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierEquals, Value: []string{"2", "3"}, Excludes: []string{"1"}}, []int{6}},
		{"depth one", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierExcludes, Value: []string{"1"}, Depth: &depthOne}, []int{3, 4, 5}},
		{"all descendants", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierExcludes, Value: []string{"1"}, Depth: &depthAll}, []int{4, 5}},
		{"missing tag", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierExcludes, Value: []string{"999"}}, []int{1, 2, 3, 4, 5, 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := sceneTagExclusionTestQuery(ctx, t, tt.c)
			ids, err := q.findIDs(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.want, ids)
		})
	}
}
