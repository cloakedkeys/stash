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
// with other letters (Été, Пример) is compared through Go's case mappings.
//
// Neither upper nor lower case alone matches what a filesystem treats as the
// same name: lower case joins İ with i, and upper case joins ı with I, while
// Windows keeps all three apart. Two names only count as the same when both
// their upper case and their lower case match. A rename that fails this is
// scanned as a new file, which is safer than handing one file's details to
// another.
//
// upper_unicode and lower_unicode call back into Go for every row, so
// pure-ASCII paths, which scans look up once per file, keep the built-in
// comparison.

func pathLike(col exp.IdentifierExpression, pattern string) exp.Expression {
	p := pathToLikePattern(pattern)
	if isASCII(pattern) {
		return goqu.L(`? LIKE ? ESCAPE '\'`, col, p)
	}
	return goqu.L(`upper_unicode(?) LIKE ? ESCAPE '\' AND lower_unicode(?) LIKE ? ESCAPE '\'`,
		col, strings.ToUpper(p), col, strings.ToLower(p))
}

func pathEqNoCase(col exp.IdentifierExpression, value string) exp.Expression {
	if isASCII(value) {
		return goqu.L("? = ? COLLATE NOCASE", col, value)
	}
	return goqu.L("upper_unicode(?) = ? AND lower_unicode(?) = ?",
		col, strings.ToUpper(value), col, strings.ToLower(value))
}
