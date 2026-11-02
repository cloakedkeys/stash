package postgres

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

// Neither upper nor lower case alone matches what a filesystem treats as the
// same name: LOWER (and ILIKE) joins İ with i, and UPPER joins ı with I, or ß
// with SS in some locales, while Windows keeps them apart. Two names only count
// as the same when both their upper case and their lower case match. A rename
// that fails this is scanned as a new file, which is safer than handing one
// file's details to another.

// Two LIKEs (upper and lower) could each match a different letter at the same
// place, so wildcards are checked letter by letter with a regular expression.
func pathLike(col exp.IdentifierExpression, pattern string) exp.Expression {
	return goqu.L(`? ~ ?`, col, pathFoldPattern(pattern))
}

// pathFoldPattern turns a path with * wildcards into a regular expression that
// ignores case letter by letter: each letter matches only the letters whose
// upper and lower case are both the same as its own (see pathEqNoCase). Its
// classes come from Go's case tables, so the database's locale doesn't matter.
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
	return goqu.L("UPPER(?) = UPPER(?) AND LOWER(?) = LOWER(?)", col, value, col, value)
}
