package sqlite

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

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
	if isASCII(pattern) {
		return goqu.L(`? LIKE ? ESCAPE '\'`, col, pathToLikePattern(pattern))
	}
	// two LIKEs (upper and lower) could each match a different letter at the
	// same place, so the check is made letter by letter instead
	// (?s): like LIKE's %, a * also matches a newline, which Linux allows in names
	return goqu.L(`? REGEXP ?`, col, "(?s)"+pathFoldPattern(pattern))
}

// pathFoldPattern turns a path with * wildcards into a regular expression that
// ignores case letter by letter: each letter matches only the letters whose
// upper and lower case are both the same as its own (see pathEqNoCase).
func pathFoldPattern(path string) string {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range path {
		if r == '*' {
			b.WriteString(".*")
			continue
		}
		variants := caseVariants(r)
		if len(variants) == 1 {
			b.WriteString(regexp.QuoteMeta(string(r)))
			continue
		}
		// only letters have variants, and no letter is special inside [ ]
		b.WriteString("[" + string(variants) + "]")
	}
	b.WriteString("$")
	return b.String()
}

func caseVariants(r rune) []rune {
	upper, lower := unicode.ToUpper(r), unicode.ToLower(r)
	ret := []rune{r}
	candidates := []rune{upper, lower, unicode.ToTitle(r)}
	for c := unicode.SimpleFold(r); c != r; c = unicode.SimpleFold(c) {
		candidates = append(candidates, c)
	}
	for _, c := range candidates {
		if !slices.Contains(ret, c) && unicode.ToUpper(c) == upper && unicode.ToLower(c) == lower {
			ret = append(ret, c)
		}
	}
	return ret
}

func pathEqNoCase(col exp.IdentifierExpression, value string) exp.Expression {
	if isASCII(value) {
		return goqu.L("? = ? COLLATE NOCASE", col, value)
	}
	return goqu.L("upper_unicode(?) = ? AND lower_unicode(?) = ?",
		col, strings.ToUpper(value), col, strings.ToLower(value))
}
