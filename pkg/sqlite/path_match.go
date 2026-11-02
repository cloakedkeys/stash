package sqlite

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

// SQLite's LIKE and NOCASE only ignore the case of ASCII letters, so a path
// with other letters (Été, Пример) goes through upper_unicode(), which compares
// upper case as Windows does. That calls back into Go for every row, so
// pure-ASCII paths, which scans look up once per file, keep the built-in
// comparison.

func pathLike(col exp.IdentifierExpression, pattern string) exp.Expression {
	if isASCII(pattern) {
		return goqu.L(`? LIKE ? ESCAPE '\'`, col, pathToLikePattern(pattern))
	}
	return goqu.L(`upper_unicode(?) LIKE ? ESCAPE '\'`, col, strings.ToUpper(pathToLikePattern(pattern)))
}

func pathEqNoCase(col exp.IdentifierExpression, value string) exp.Expression {
	if isASCII(value) {
		return goqu.L("? = ? COLLATE NOCASE", col, value)
	}
	return goqu.L("upper_unicode(?) = ?", col, strings.ToUpper(value))
}
