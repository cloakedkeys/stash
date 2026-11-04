package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStableSort(t *testing.T) {
	for _, tt := range []struct {
		sort, want string
	}{
		{"", " ORDER BY scenes.id ASC"},
		{"  ", " ORDER BY scenes.id ASC"},
		{" ORDER BY scenes.date DESC NULLS LAST", " ORDER BY scenes.date DESC NULLS LAST, scenes.id ASC"},
		// once a sort reaches a name, same names come back newest first when
		// the name is ascending, as on SQLite
		{` ORDER BY scenes.date ASC NULLS LAST, COALESCE(scenes.title, CAST(scenes.id as text)) COLLATE "natural_ci" ASC`,
			` ORDER BY scenes.date ASC NULLS LAST, COALESCE(scenes.title, CAST(scenes.id as text)) COLLATE "natural_ci" ASC, scenes.id DESC`},
		{` ORDER BY scenes.title COLLATE "natural_ci" ASC NULLS LAST`, ` ORDER BY scenes.title COLLATE "natural_ci" ASC NULLS LAST, scenes.id DESC`},
		{` ORDER BY scenes.title COLLATE "natural_ci" asc`, ` ORDER BY scenes.title COLLATE "natural_ci" asc, scenes.id DESC`},
		{` ORDER BY scenes.title COLLATE "natural_ci"`, ` ORDER BY scenes.title COLLATE "natural_ci", scenes.id DESC`},
		// only the first name counts
		{` ORDER BY scenes.title COLLATE "natural_ci" DESC, folders.path COLLATE "natural_ci" DESC, COALESCE(scenes.title, CAST(scenes.id as text)) COLLATE "natural_ci" ASC`,
			` ORDER BY scenes.title COLLATE "natural_ci" DESC, folders.path COLLATE "natural_ci" DESC, COALESCE(scenes.title, CAST(scenes.id as text)) COLLATE "natural_ci" ASC, scenes.id ASC`},
		{` ORDER BY scenes.title COLLATE "natural_ci" desc NULLS LAST`, ` ORDER BY scenes.title COLLATE "natural_ci" desc NULLS LAST, scenes.id ASC`},
	} {
		qb := queryBuilder{sort: tt.sort}
		distinctIDs(&qb, "scenes")
		assert.Equal(t, tt.want, qb.stableSort(), "sort %q", tt.sort)
	}

	// a query with no item id keeps its sort as it is
	qb := queryBuilder{sort: " ORDER BY x ASC"}
	assert.Equal(t, " ORDER BY x ASC", qb.stableSort())
}
