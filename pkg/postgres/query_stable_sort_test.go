package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStableSort(t *testing.T) {
	const finalTitle = `COALESCE(scenes.title, CAST(scenes.id as text)) COLLATE "natural_ci" ASC`
	const finalTitleWhen = ` WHEN COALESCE(scenes.title, CAST(scenes.id as text)) IS NOT NULL THEN -scenes.id`

	for _, tt := range []struct {
		sort, want string
	}{
		// no sort: id order
		{"", " ORDER BY scenes.id ASC"},
		{"  ", " ORDER BY scenes.id ASC"},
		// no name key: id order
		{" ORDER BY scenes.date DESC NULLS LAST", " ORDER BY scenes.date DESC NULLS LAST, scenes.id ASC"},
		// once a sort reaches a name with a value, same names come back
		// newest first when it is ascending, as on SQLite
		{" ORDER BY scenes.date ASC NULLS LAST, " + finalTitle,
			" ORDER BY scenes.date ASC NULLS LAST, " + finalTitle + ", CASE" + finalTitleWhen + " ELSE scenes.id END ASC"},
		{` ORDER BY scenes.title COLLATE "natural_ci" asc`,
			` ORDER BY scenes.title COLLATE "natural_ci" asc, CASE WHEN scenes.title IS NOT NULL THEN -scenes.id ELSE scenes.id END ASC`},
		{` ORDER BY scenes.title COLLATE "natural_ci"`,
			` ORDER BY scenes.title COLLATE "natural_ci", CASE WHEN scenes.title IS NOT NULL THEN -scenes.id ELSE scenes.id END ASC`},
		// a descending name gives oldest first; an empty one moves on to
		// the next name key, as SQLite does for scenes with no studio
		{` ORDER BY studios.name COLLATE "natural_ci" DESC NULLS LAST, ` + finalTitle,
			` ORDER BY studios.name COLLATE "natural_ci" DESC NULLS LAST, ` + finalTitle + `, CASE WHEN studios.name IS NOT NULL THEN scenes.id` + finalTitleWhen + ` ELSE scenes.id END ASC`},
		// commas inside brackets and quotes don't split keys
		{` ORDER BY COALESCE(folders.path, '') || ',' || COALESCE(files.basename, '') COLLATE "natural_ci" desc`,
			` ORDER BY COALESCE(folders.path, '') || ',' || COALESCE(files.basename, '') COLLATE "natural_ci" desc, CASE WHEN COALESCE(folders.path, '') || ',' || COALESCE(files.basename, '') IS NOT NULL THEN scenes.id ELSE scenes.id END ASC`},
	} {
		qb := queryBuilder{sort: tt.sort}
		distinctIDs(&qb, "scenes")
		assert.Equal(t, tt.want, qb.stableSort(), "sort %q", tt.sort)
	}

	// a query with no item id keeps its sort as it is
	qb := queryBuilder{sort: " ORDER BY x ASC"}
	assert.Equal(t, " ORDER BY x ASC", qb.stableSort())
}
