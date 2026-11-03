//go:build integration

package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func containsLiteralDatabase(t *testing.T) (func(string, ...interface{}), func(string, ...interface{}) []int) {
	t.Helper()
	db, err := sqlx.Open(sqlite3Driver, ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return func(query string, args ...interface{}) {
			t.Helper()
			_, err := db.Exec(query, args...)
			require.NoError(t, err)
		}, func(query string, args ...interface{}) []int {
			t.Helper()
			var ids []int
			require.NoError(t, db.Select(&ids, query, args...))
			return ids
		}
}

func TestContainsLiteralText(t *testing.T) {
	execSQL, selectIDs := containsLiteralDatabase(t)
	execSQL("CREATE TEMP TABLE literal_items (id INTEGER PRIMARY KEY, value TEXT, path TEXT, basename TEXT)")
	execSQL("CREATE TEMP TABLE literal_strings (item_id INTEGER, value TEXT)")
	execSQL("CREATE TEMP TABLE literal_custom (item_id INTEGER, field TEXT, value TEXT)")
	values := []string{"my part_one clip", "my part-one clip", "my partXone clip", "50% done", "500 things", `my part\one clip`, "my partone clip", "my PART_ONE clip", "part_one other clip"}
	for i, value := range values {
		execSQL("INSERT INTO literal_items VALUES (?, ?, ?, ?)", i+1, value, "library", value)
		execSQL("INSERT INTO literal_strings VALUES (?, ?)", i+1, value)
		execSQL("INSERT INTO literal_custom VALUES (?, ?, ?)", i+1, "text", value)
	}
	execSQL("INSERT INTO literal_items VALUES (10, NULL, NULL, NULL)")

	tests := []struct {
		name  string
		value string
		want  []int
	}{
		{"underscore", "part_one", []int{1, 8, 9}},
		{"percent", "50%", []int{4}},
		{"backslash", `part\one`, []int{6}},
		{"case", "PART_ONE", []int{1, 8, 9}},
		{"words", "part_one missing", []int{1, 8, 9}},
		{"phrase", `"part_one clip"`, []int{1, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, excludes := range []bool{false, true} {
				modifier := models.CriterionModifierIncludes
				want := tt.want
				if excludes {
					modifier = models.CriterionModifierExcludes
					want = containsLiteralComplement(tt.want, 10)
				}
				c := &models.StringCriterionInput{Modifier: modifier, Value: tt.value}
				f := &filterBuilder{}
				stringCriterionHandler(c, "literal_items.value")(context.Background(), f)
				where, args := f.generateWhereClauses()
				require.Equal(t, want, selectIDs("SELECT id FROM literal_items WHERE "+where+" ORDER BY id", args...), "excludes=%v", excludes)

				// NULL paths are not retained by a path exclusion.
				f = &filterBuilder{}
				pathCriterionHandler(c, "literal_items.path", "literal_items.basename", nil)(context.Background(), f)
				where, args = f.generateWhereClauses()
				pathWant := want
				if excludes {
					pathWant = containsLiteralComplement(tt.want, 9)
				}
				require.Equal(t, pathWant, selectIDs("SELECT id FROM literal_items WHERE "+where+" ORDER BY id", args...), "path excludes=%v", excludes)
			}
		})
	}

	for _, tt := range tests[:4] {
		t.Run("search/"+tt.name, func(t *testing.T) {
			q := &queryBuilder{}
			q.parseQueryString([]string{"literal_items.value"}, tt.value)
			require.Equal(t, tt.want, selectIDs("SELECT id FROM literal_items WHERE "+strings.Join(q.whereClauses, " AND ")+" ORDER BY id", q.whereArgs...))
		})
		for _, excludes := range []bool{false, true} {
			modifier := models.CriterionModifierIncludes
			want := tt.want
			if excludes {
				modifier = models.CriterionModifierExcludes
				want = containsLiteralComplement(tt.want, 10)
			}
			h := &customFieldsFilterHandler{table: "literal_custom", fkCol: "item_id", idCol: "literal_items.id"}
			f := &filterBuilder{}
			h.handleCriterion(f, "cf", models.CustomFieldCriterionInput{Field: "text", Modifier: modifier, Value: []interface{}{tt.value}})
			require.NoError(t, f.getError())
			where, args := f.generateWhereClauses()
			joins := f.getAllJoins()
			var joinArgs []interface{}
			for _, j := range joins {
				joinArgs = append(joinArgs, j.args...)
			}
			args = append(joinArgs, args...)
			require.Equal(t, want, selectIDs("SELECT literal_items.id FROM literal_items"+joins.toSQL(false)+" WHERE "+where+" ORDER BY literal_items.id", args...), "custom %s excludes=%v", tt.name, excludes)
		}
		f := &filterBuilder{}
		m := &stringListCriterionHandlerBuilder{primaryTable: "literal_items", primaryFK: "item_id", joinTable: "literal_strings", stringColumn: "value"}
		m.handler(&models.StringCriterionInput{Modifier: models.CriterionModifierExcludes, Value: tt.value})(context.Background(), f)
		where, args := f.generateWhereClauses()
		require.Equal(t, containsLiteralComplement(tt.want, 10), selectIDs("SELECT id FROM literal_items WHERE "+where+" ORDER BY id", args...), "string list %s", tt.name)
	}
}

func containsLiteralComplement(matches []int, count int) []int {
	var ret []int
	for id := 1; id <= count; id++ {
		found := false
		for _, match := range matches {
			found = found || id == match
		}
		if !found {
			ret = append(ret, id)
		}
	}
	return ret
}

func TestCaptionExcludesLiteralText(t *testing.T) {
	execSQL, selectIDs := containsLiteralDatabase(t)
	execSQL("CREATE TEMP TABLE scenes (id INTEGER PRIMARY KEY)")
	execSQL("CREATE TEMP TABLE files (id INTEGER PRIMARY KEY)")
	execSQL("CREATE TEMP TABLE scenes_files (scene_id INTEGER, file_id INTEGER)")
	execSQL("CREATE TEMP TABLE video_captions (file_id INTEGER, language_code TEXT)")
	for i, code := range []string{"part_one", "part-one", "partXone", "50%", "500", `part\one`, "partone"} {
		execSQL("INSERT INTO scenes VALUES (?)", i+1)
		execSQL("INSERT INTO files VALUES (?)", i+1)
		execSQL("INSERT INTO scenes_files VALUES (?, ?)", i+1, i+1)
		execSQL("INSERT INTO video_captions VALUES (?, ?)", i+1, code)
	}
	for _, tt := range []struct {
		value string
		id    int
	}{
		{"PART_ONE", 1},
		{"50%", 4},
		{`part\one`, 6},
	} {
		c := &models.StringCriterionInput{Modifier: models.CriterionModifierExcludes, Value: tt.value}
		f := &filterBuilder{}
		(&sceneFilterHandler{}).captionCriterionHandler(c)(context.Background(), f)
		where, args := f.generateWhereClauses()
		want := containsLiteralComplement([]int{tt.id}, 7)
		require.Equal(t, want, selectIDs("SELECT id FROM scenes WHERE "+where+" ORDER BY id", args...))
		f = &filterBuilder{}
		(&videoFileFilterHandler{}).captionCriterionHandler(c)(context.Background(), f)
		where, args = f.generateWhereClauses()
		require.Equal(t, want, selectIDs("SELECT id FROM files WHERE "+where+" ORDER BY id", args...))
	}
}
