package postgres

import (
	"strings"
	"sync"
	"unicode"
)

// SQLite sorts names and titles with its NATURAL_CI collation, which is
// casefolded.NaturalLess from github.com/WithoutPants/sortorder. PostgreSQL's
// natural_ci ICU collation puts names in a different order: it orders letters
// and punctuation by language rules rather than by character code, and keeps
// names that differ only in case apart. So a name sort on PostgreSQL instead
// orders by a sort key that compares, byte by byte, the way NaturalLess
// compares the names:
//
//   - every character becomes its case-folded form, the lowest character it is
//     equal to ignoring case, as NaturalLess compares them. That is the
//     capital for ASCII letters.
//   - a run of digits becomes the length of the number without its leading
//     zeros, as five digits, then the number, then the byte position in the
//     name where the number's leading zeros end, as ten digits. So longer
//     numbers sort after shorter ones, and since the key starts with a digit,
//     a number still sorts against other characters the way its first digit
//     does.
//
// Equal numbers are ordered by that byte position because NaturalLess does so.
// Its comments call this ordering by the number of leading zeros, which it is
// when the names before the number are the same bytes. When they are equal
// only ignoring case, and their characters have different lengths in UTF-8,
// such as "k" and the Kelvin sign (U+212A, three bytes), the position differs
// too: "k0" sorts before the Kelvin sign followed by "0". So the digits are found in the name as it is, and each run of
// other characters is folded on its own; no character folds to or from a digit.
//
// Compared with the "C" collation, UTF-8 sorts by character code, as
// NaturalLess does. Names whose numbers have 100000 digits or more, or that are
// a gigabyte or longer, are the only ones whose key doesn't match.

// naturalFoldCTE names the common table expression that holds the case-folding
// tables, so a query holds them once and passes them as arguments.
const naturalFoldCTE = "natural_fold"

// naturalFold holds the case-folding tables, built once from Go's Unicode
// tables, which are the ones NaturalLess uses.
type naturalFold struct {
	// cased is a regular expression matching any character other than ASCII
	// that case folding changes. Names with none are folded with upper,
	// which is quicker than translate.
	cased string
	// from and to are translate's arguments that fold every character.
	from, to string
}

var naturalFoldTables = sync.OnceValue(func() naturalFold {
	var from, to strings.Builder
	add := func(r rune) {
		from.WriteRune(r)
		to.WriteRune(caseFold(r))
	}
	// translate looks a character up from the start, so the common ones go
	// first, even though most don't change
	for r := rune(' '); r <= '~'; r++ {
		add(r)
	}
	for r := rune(0x80); r <= 0xFF; r++ {
		add(r)
	}
	for r := rune(0x2010); r <= 0x2027; r++ {
		add(r)
	}

	var cased strings.Builder
	cased.WriteString("[")
	var start, prev rune = -1, -1
	flush := func() {
		if start < 0 {
			return
		}
		cased.WriteRune(start)
		if prev > start {
			if prev > start+1 {
				cased.WriteString("-")
			}
			cased.WriteRune(prev)
		}
	}
	for r := rune(0x80); r <= unicode.MaxRune; r++ {
		if caseFold(r) == r {
			continue
		}
		if r > 0xFF {
			add(r)
		}
		if r != prev+1 {
			flush()
			start = r
		}
		prev = r
	}
	flush()
	cased.WriteString("]")

	return naturalFold{cased: cased.String(), from: from.String(), to: to.String()}
})

// caseFold returns the lowest character equal to r ignoring case, as
// NaturalLess compares characters.
func caseFold(r rune) rune {
	var prev rune = -1
	for r > prev {
		prev, r = r, unicode.SimpleFold(r)
	}
	return r
}

// naturalFoldWith returns the common table expression with the folding
// tables, and its arguments.
func naturalFoldWith() (string, []interface{}) {
	t := naturalFoldTables()
	return naturalFoldCTE + " AS (SELECT CAST(? AS text) AS cased, CAST(? AS text) AS fold_from, CAST(? AS text) AS fold_to)",
		[]interface{}{t.cased, t.from, t.to}
}

// naturalSortKey returns the sort key of the text expression expr, which
// sorts with the "C" collation as SQLite's NATURAL_CI collation sorts expr.
// It is NULL when expr is. The query needs the natural_fold table.
func naturalSortKey(expr string) string {
	// a run of characters other than digits, folded
	const text = "COALESCE(m[3], '')"
	const fold = "CASE WHEN " + text + " ~ (SELECT cased FROM " + naturalFoldCTE + ")" +
		" THEN translate(" + text + ", (SELECT fold_from FROM " + naturalFoldCTE + "), (SELECT fold_to FROM " + naturalFoldCTE + "))" +
		` ELSE upper(` + text + ` COLLATE "C") END`
	// where a number's leading zeros end, counting bytes from 0: NaturalLess
	// eats every zero of a number that is all zeros
	const nonZero = "(start + octet_length(m[1]) + CASE WHEN m[2] = '0' THEN 1 ELSE 0 END)"
	// Each match is a number, with its leading zeros apart, or a run of other
	// characters; ^$ matches an empty name, so it gives an empty key. start is
	// the byte where the match begins.
	return "(SELECT string_agg(CASE WHEN m[2] IS NULL THEN " + fold +
		" ELSE lpad(CAST(length(m[2]) AS text), 5, '0') || m[2] || lpad(CAST(" + nonZero + " AS text), 10, '0') END, '' ORDER BY o)" +
		" FROM (SELECT m, o, COALESCE(sum(octet_length(COALESCE(m[1], '') || COALESCE(m[2], '') || COALESCE(m[3], '')))" +
		" OVER (ORDER BY o ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING), 0) AS start" +
		" FROM regexp_matches(" + expr + ", '(0*)(0|[123456789][0123456789]*)|([^0123456789]+)|^$', 'g') WITH ORDINALITY AS natural_match(m, o)) AS natural_part)" +
		` COLLATE "C"`
}
