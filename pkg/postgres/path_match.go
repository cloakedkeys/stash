package postgres

import (
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

func pathHasWildcard(path string) bool {
	return strings.Contains(path, "*")
}

func pathToLikePattern(path string) string {
	var b strings.Builder
	b.Grow(len(path))

	for _, r := range path {
		switch r {
		case '\\', '%', '_':
			b.WriteRune('\\')
		case '*':
			b.WriteRune('%')
			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}

// Neither upper nor lower case alone matches what a filesystem treats as the
// same name: LOWER (and ILIKE) joins İ with i, and UPPER joins ı with I, or ß
// with SS in some locales, while Windows keeps them apart. Two names only count
// as the same when both their upper case and their lower case match. A rename
// that fails this is scanned as a new file, which is safer than handing one
// file's details to another.

func pathLike(col exp.IdentifierExpression, pattern string) exp.Expression {
	p := pathToLikePattern(pattern)
	return goqu.L(`UPPER(?) LIKE UPPER(?) ESCAPE '\' AND LOWER(?) LIKE LOWER(?) ESCAPE '\'`, col, p, col, p)
}

func pathEqNoCase(col exp.IdentifierExpression, value string) exp.Expression {
	return goqu.L("UPPER(?) = UPPER(?) AND LOWER(?) = LOWER(?)", col, value, col, value)
}
