package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func performerStudioTestContext(t *testing.T) context.Context {
	t.Helper()
	db, err := sqlx.Open(sqlite3Driver, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	// The studio_id indexes match the real schema: images' is partial.
	_, err = db.Exec(`CREATE TABLE performers (id INTEGER PRIMARY KEY);
CREATE TABLE studios (id INTEGER PRIMARY KEY, parent_id INTEGER);
CREATE TABLE scenes (id INTEGER PRIMARY KEY, studio_id INTEGER);
CREATE TABLE images (id INTEGER PRIMARY KEY, studio_id INTEGER);
CREATE TABLE galleries (id INTEGER PRIMARY KEY, studio_id INTEGER);
CREATE TABLE performers_scenes (performer_id INTEGER, scene_id INTEGER, PRIMARY KEY(scene_id, performer_id));
CREATE TABLE performers_images (performer_id INTEGER, image_id INTEGER, PRIMARY KEY(image_id, performer_id));
CREATE TABLE performers_galleries (performer_id INTEGER, gallery_id INTEGER, PRIMARY KEY(gallery_id, performer_id));
CREATE INDEX index_scenes_on_studio_id ON scenes (studio_id);
CREATE INDEX index_images_on_studio_id ON images (studio_id) WHERE studio_id IS NOT NULL;
CREATE INDEX index_galleries_on_studio_id ON galleries (studio_id);`)
	require.NoError(t, err)
	tx, err := db.Beginx()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	return context.WithValue(context.Background(), txnKey, tx)
}

func performerStudioTestQuery(ctx context.Context, t *testing.T, c models.HierarchicalMultiCriterionInput) queryBuilder {
	t.Helper()
	f := &filterBuilder{}
	(&performerFilterHandler{}).studiosCriterionHandler(&c)(ctx, f)
	q := performerRepository.newQuery()
	q.columns = []string{"DISTINCT performers.id"}
	q.from = performerTable
	q.sortAndPagination = " ORDER BY performers.id"
	require.NoError(t, q.addFilter(f))
	return q
}

func performerStudioTestPlan(ctx context.Context, t *testing.T, q queryBuilder) string {
	t.Helper()
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
	return strings.Join(details, "\n")
}

// #1656: the per-studio performer count on the Studios page must not build a
// throwaway index over every image when the image statistics are out of date.
func TestPerformerStudioFilterImagePlan(t *testing.T) {
	ctx := performerStudioTestContext(t)
	// 20,000 images, of which 2,000 are spread over 200 studios.
	_, err := dbWrapper.Exec(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
INSERT INTO images SELECT x, CASE WHEN x<=2000 THEN 1 + x % 200 END FROM n;
INSERT INTO studios SELECT DISTINCT studio_id, NULL FROM images WHERE studio_id IS NOT NULL;
INSERT INTO performers SELECT id FROM images WHERE id<=500;
INSERT INTO performers_images SELECT 1 + id % 500, id FROM images;
ANALYZE;`)
	require.NoError(t, err)

	criteria := []models.HierarchicalMultiCriterionInput{
		{Modifier: models.CriterionModifierIncludes, Value: []string{"7"}},
		{Modifier: models.CriterionModifierIncludes, Value: []string{"8"}, Excludes: []string{"7"}},
	}

	t.Run("fresh statistics use the studio index", func(t *testing.T) {
		for _, c := range criteria {
			plan := performerStudioTestPlan(ctx, t, performerStudioTestQuery(ctx, t, c))
			require.Contains(t, plan, "images USING COVERING INDEX index_images_on_studio_id", "look up the studio's images:\n%s", plan)
		}
	})

	t.Run("stale statistics don't build a throwaway index", func(t *testing.T) {
		// What ANALYZE leaves behind when it ran while every image with a studio
		// had the same one. Adding studios later doesn't change the row count, so
		// the automatic optimize never refreshes it.
		_, err := dbWrapper.Exec(ctx, `UPDATE sqlite_stat1 SET stat = '2000 2000' WHERE idx = 'index_images_on_studio_id';
ANALYZE sqlite_schema;`)
		require.NoError(t, err)
		for _, c := range criteria {
			plan := performerStudioTestPlan(ctx, t, performerStudioTestQuery(ctx, t, c))
			require.NotContains(t, plan, "images USING AUTOMATIC", "must not index every image for each studio:\n%s", plan)
		}
	})
}

func TestPerformerStudioFilterCounts(t *testing.T) {
	ctx := performerStudioTestContext(t)
	// Performer 1 only in a scene of studio 1, 2 only in an image of studio 1,
	// 3 only in a gallery of studio 1, 4 in an image of studio 2 (a child of 1),
	// 5 in an image with no studio, 6 in nothing.
	_, err := dbWrapper.Exec(ctx, `INSERT INTO performers VALUES (1),(2),(3),(4),(5),(6);
INSERT INTO studios VALUES (1, NULL),(2, 1),(3, NULL);
INSERT INTO scenes VALUES (1, 1);
INSERT INTO images VALUES (1, 1),(2, 2),(3, NULL);
INSERT INTO galleries VALUES (1, 1);
INSERT INTO performers_scenes VALUES (1, 1);
INSERT INTO performers_images VALUES (2, 1),(4, 2),(5, 3);
INSERT INTO performers_galleries VALUES (3, 1);`)
	require.NoError(t, err)
	all := -1
	tests := []struct {
		name string
		c    models.HierarchicalMultiCriterionInput
		want []int
	}{
		{"includes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierIncludes, Value: []string{"1"}}, []int{1, 2, 3}},
		{"includes with sub-studios", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierIncludes, Value: []string{"1"}, Depth: &all}, []int{1, 2, 3, 4}},
		{"excludes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierExcludes, Value: []string{"1"}}, []int{4, 5, 6}},
		{"includes and excludes", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierIncludes, Value: []string{"1"}, Depth: &all, Excludes: []string{"2"}}, []int{1, 2, 3}},
		{"studio with nothing", models.HierarchicalMultiCriterionInput{Modifier: models.CriterionModifierIncludes, Value: []string{"3"}}, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := performerStudioTestQuery(ctx, t, tt.c).findIDs(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.want, ids)
		})
	}
}
