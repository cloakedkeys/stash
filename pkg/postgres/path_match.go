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

// Names compare by their upper case, as Windows does. Lower case (and ILIKE)
// would join names Windows keeps apart, such as ai.txt and aİ.txt.

func pathLike(col exp.IdentifierExpression, pattern string) exp.Expression {
	return goqu.L(`UPPER(?) LIKE UPPER(?) ESCAPE '\'`, col, pathToLikePattern(pattern))
}

func pathEqNoCase(col exp.IdentifierExpression, value string) exp.Expression {
	return goqu.L("UPPER(?) = UPPER(?)", col, value)
}
