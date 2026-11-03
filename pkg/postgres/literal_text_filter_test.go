//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

// Execute the clauses produced by the real handlers against throwaway rows.
func TestLiteralTextCriteria(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	db, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close(ctx)) })
	check := func(t *testing.T, handler criterionHandlerFunc, value, path, basename, folder string, want bool) {
		t.Helper()
		f := &filterBuilder{}
		handler(ctx, f)
		require.NoError(t, f.getError())
		require.Len(t, f.whereClauses, 1)
		clause := f.whereClauses[0]
		query := "SELECT EXISTS (SELECT 1 FROM (SELECT CAST(? AS TEXT) AS value, CAST(? AS TEXT) AS path) AS folders CROSS JOIN (SELECT CAST(? AS TEXT) AS basename) AS files CROSS JOIN (SELECT CAST(? AS TEXT) AS path) AS gallery_folder WHERE " + clause.sql + ")"
		args := append([]interface{}{value, path, basename, folder}, clause.args...)
		// pgx takes $1, $2, … placeholders
		parts := strings.Split(query, "?")
		var numbered strings.Builder
		for i, part := range parts {
			if i > 0 {
				fmt.Fprintf(&numbered, "$%d", i)
			}
			numbered.WriteString(part)
		}
		query = numbered.String()
		var got bool
		require.NoError(t, db.QueryRow(ctx, query, args...).Scan(&got))
		require.Equal(t, want, got)
	}

	for _, tc := range []struct {
		name      string
		value     string
		candidate string
		matches   bool
	}{
		{"underscore literal", "part_one", "part_one", true},
		{"underscore wildcard", "part_one", "part-one", false},
		{"percent literal", "100%", "100%", true},
		{"percent wildcard", "100%", "1000 things", false},
		{"backslash literal", `part\one`, `part\one`, true},
		{"backslash preserved", `part\one`, "partone", false},
		{"case insensitive", "part_one", "PART_ONE", true},
		{"empty", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, modifier := range []models.CriterionModifier{models.CriterionModifierEquals, models.CriterionModifierNotEquals} {
				c := &models.StringCriterionInput{Value: tc.value, Modifier: modifier}
				want := tc.matches
				if modifier == models.CriterionModifierNotEquals {
					want = !want
				}
				check(t, stringCriterionHandler(c, "folders.value"), tc.candidate, "", "", "", want)
				check(t, uuidCriterionHandler(c, "folders.value"), tc.candidate, "", "", "", want)
				pathValue := filepath.Join("root", "texts", tc.value)
				candidate := filepath.Join("root", "texts", tc.candidate)
				pc := &models.StringCriterionInput{Value: pathValue, Modifier: modifier}
				check(t, pathCriterionHandler(pc, "folders.path", "files.basename", nil), "", filepath.Dir(candidate), filepath.Base(candidate), "", want)
				check(t, (&galleryFilterHandler{}).pathCriterionHandler(pc), "", filepath.Dir(candidate), filepath.Base(candidate), candidate, want)
				if modifier == models.CriterionModifierEquals {
					// Folder galleries also match without an associated file.
					check(t, (&galleryFilterHandler{}).pathCriterionHandler(c), "", "", "", tc.candidate, want)
				}
			}
		})
	}

	for _, folder := range []string{"my_dir", "my%dir", `my\dir`} {
		for _, modifier := range []models.CriterionModifier{models.CriterionModifierEquals, models.CriterionModifierNotEquals} {
			prefix := filepath.Join("root", folder) + string(filepath.Separator)
			c := &models.StringCriterionInput{Value: prefix, Modifier: modifier, MatchPrefix: true}
			for _, candidate := range []string{folder, "my-dir"} {
				want := candidate == folder
				if modifier == models.CriterionModifierNotEquals {
					want = !want
				}
				path := filepath.Join("root", candidate)
				check(t, pathCriterionHandler(c, "folders.path", "files.basename", nil), "", path, "clip.mp4", "", want)
				check(t, stringCriterionHandler(c, "folders.value"), filepath.Join(path, "clip.mp4"), "", "", "", want)
				if modifier == models.CriterionModifierEquals {
					check(t, (&galleryFilterHandler{}).pathCriterionHandler(c), "", path, "clip.zip", "", want)
					check(t, (&galleryFilterHandler{}).pathCriterionHandler(c), "", "", "", filepath.Join(path, "child"), want)
				}
			}
		}
	}
}
