package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStableSort(t *testing.T) {
	const finalTitle = `scenes.title COLLATE "natural_ci" ASC`
	const finalTitleWhen = ` WHEN (scenes.title) IS NOT NULL THEN -scenes.id`
	title := naturalSortKey("scenes.title")
	studio := naturalSortKey("studios.name")
	// a key after a name key is only used when the name is empty, as on SQLite
	afterStudio := func(expr string) string { return "CASE WHEN (studios.name) IS NULL THEN " + expr + " END" }
	const path = `COALESCE(folders.path, '') || ',' || COALESCE(files.basename, '')`

	for _, tt := range []struct {
		sort, want string
	}{
		// no sort: id order
		{"", " ORDER BY scenes.id ASC"},
		{"  ", " ORDER BY scenes.id ASC"},
		// no name key: id order; empty values last when descending, as on SQLite
		{" ORDER BY scenes.date DESC NULLS LAST", " ORDER BY scenes.date DESC NULLS LAST, scenes.id ASC"},
		// empty values first when ascending, as on SQLite, whatever the sort asked
		{" ORDER BY scenes.date ASC NULLS LAST", " ORDER BY scenes.date ASC NULLS FIRST, scenes.id ASC"},
		{" ORDER BY scenes.rating DESC", " ORDER BY scenes.rating DESC NULLS LAST, scenes.id ASC"},
		// names sort by their natural sort key; once a sort reaches a name with
		// a value, same names come back newest first when it is ascending
		{" ORDER BY scenes.date ASC NULLS LAST, " + finalTitle,
			" ORDER BY scenes.date ASC NULLS FIRST, " + title + " ASC NULLS FIRST, CASE" + finalTitleWhen + " ELSE scenes.id END ASC"},
		{` ORDER BY scenes.title COLLATE "natural_ci" asc`,
			" ORDER BY " + title + " ASC NULLS FIRST, CASE" + finalTitleWhen + " ELSE scenes.id END ASC"},
		{` ORDER BY scenes.title COLLATE "natural_ci"`,
			" ORDER BY " + title + " ASC NULLS FIRST, CASE" + finalTitleWhen + " ELSE scenes.id END ASC"},
		// a descending name gives oldest first; an empty one moves on to the
		// next key, as SQLite does for scenes with no studio
		{` ORDER BY studios.name COLLATE "natural_ci" DESC NULLS LAST, ` + finalTitle,
			" ORDER BY " + studio + " DESC NULLS LAST, " + afterStudio(title) + ` COLLATE "C" ASC NULLS FIRST, CASE WHEN (studios.name) IS NOT NULL THEN scenes.id` + finalTitleWhen + " ELSE scenes.id END ASC"},
		// keys that aren't names are skipped too once a name has a value
		{` ORDER BY studios.name COLLATE "natural_ci" ASC, scenes.date DESC`,
			" ORDER BY " + studio + " ASC NULLS FIRST, " + afterStudio("scenes.date") + " DESC NULLS LAST, CASE WHEN (studios.name) IS NOT NULL THEN -scenes.id ELSE scenes.id END ASC"},
		// commas inside brackets and quotes don't split keys
		{` ORDER BY ` + path + ` COLLATE "natural_ci" desc`,
			" ORDER BY " + naturalSortKey(path) + " DESC NULLS LAST, CASE WHEN (" + path + ") IS NOT NULL THEN scenes.id ELSE scenes.id END ASC"},
		// a column whose name ends in asc or desc keeps it
		{" ORDER BY files.basc", " ORDER BY files.basc ASC NULLS FIRST, scenes.id ASC"},
		{" ORDER BY files.basc DESC", " ORDER BY files.basc DESC NULLS LAST, scenes.id ASC"},
	} {
		qb := queryBuilder{sort: tt.sort}
		distinctIDs(&qb, "scenes")
		assert.Equal(t, tt.want, qb.stableSort(), "sort %q", tt.sort)
	}

	// a query with no item id keeps its sort as it is
	qb := queryBuilder{sort: " ORDER BY x ASC"}
	assert.Equal(t, " ORDER BY x ASC", qb.stableSort())
}

func TestCutSuffixFold(t *testing.T) {
	for _, tt := range []struct {
		s, suffix, want string
		ok              bool
	}{
		{"scenes.title DESC", "DESC", "scenes.title", true},
		{"scenes.title desc", "DESC", "scenes.title", true},
		{"scenes.title  ASC", "ASC", "scenes.title", true},
		{"files.basc", "ASC", "files.basc", false},
		{"ASC", "ASC", "ASC", false},
		{`x COLLATE "natural_ci"`, `COLLATE "natural_ci"`, "x", true},
	} {
		got, ok := cutSuffixFold(tt.s, tt.suffix)
		assert.Equal(t, tt.want, got, "%q", tt.s)
		assert.Equal(t, tt.ok, ok, "%q", tt.s)
	}
}
