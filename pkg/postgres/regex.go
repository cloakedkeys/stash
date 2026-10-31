package postgres

import (
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"
	"unicode"
)

// maxTranslatedRegexLen limits how large a translated pattern can get. Repeats are written
// out in full, so a short pattern such as `\pL{1000}` can translate to megabytes.
const maxTranslatedRegexLen = 256 * 1024

var errRegexTooComplex = errors.New("regular expression is too complex")

const (
	// never matches: nothing can follow the end of the text
	regexNoMatch = `\Za`
	regexWord    = `[0-9A-Za-z_]`
)

// translateRegex rewrites a pattern in Go's regular expression syntax, which is what Stash
// accepts on every database, into an equivalent one for PostgreSQL's ~ operator.
//
// It's only for testing whether a string matches. Greediness and capture groups can't change
// that, so they aren't kept.
//
// The result doesn't rely on the database's locale: character classes and case-insensitive
// letters are written out as explicit characters, following Go's rules for them.
func translateRegex(pattern string) (string, error) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", err
	}

	// replaces repeats, which PostgreSQL limits to 255, with their expanded form
	re = re.Simplify()

	var b strings.Builder
	if err := writeRegex(&b, re); err != nil {
		return "", fmt.Errorf("%w: %s", err, pattern)
	}
	return b.String(), nil
}

func writeRegexChar(b *strings.Builder, r rune) {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == ' ', r == '_', r == '/':
		b.WriteRune(r)
	case r <= 0xFFFF:
		fmt.Fprintf(b, `\u%04x`, r)
	default:
		fmt.Fprintf(b, `\U%08x`, r)
	}
}

func writeRegexLiteral(b *strings.Builder, re *syntax.Regexp) {
	for _, r := range re.Rune {
		switch {
		case r == 0:
			// PostgreSQL text can't hold this character, so nothing can match it
			b.WriteString(regexNoMatch)
		case re.Flags&syntax.FoldCase != 0 && unicode.SimpleFold(r) != r:
			b.WriteByte('[')
			writeRegexChar(b, r)
			for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
				writeRegexChar(b, f)
			}
			b.WriteByte(']')
		default:
			writeRegexChar(b, r)
		}
	}
}

func writeRegexClass(b *strings.Builder, ranges []rune) {
	var class strings.Builder
	for i := 0; i+1 < len(ranges); i += 2 {
		lo, hi := ranges[i], ranges[i+1]
		// PostgreSQL text can't hold character 0, and it's not valid in a pattern
		if lo == 0 {
			lo = 1
		}
		if hi < lo {
			continue
		}
		writeRegexChar(&class, lo)
		if hi > lo {
			class.WriteByte('-')
			writeRegexChar(&class, hi)
		}
	}

	if class.Len() == 0 {
		b.WriteString(regexNoMatch)
		return
	}
	b.WriteByte('[')
	b.WriteString(class.String())
	b.WriteByte(']')
}

func writeRegexGroup(b *strings.Builder, re *syntax.Regexp, suffix string) error {
	b.WriteString("(?:")
	if err := writeRegex(b, re); err != nil {
		return err
	}
	b.WriteString(")" + suffix)
	return nil
}

func writeRegex(b *strings.Builder, re *syntax.Regexp) error {
	if b.Len() > maxTranslatedRegexLen {
		return errRegexTooComplex
	}

	switch re.Op {
	case syntax.OpNoMatch:
		b.WriteString(regexNoMatch)
	case syntax.OpEmptyMatch:
		b.WriteString("(?:)")
	case syntax.OpLiteral:
		writeRegexLiteral(b, re)
	case syntax.OpCharClass:
		writeRegexClass(b, re.Rune)
	case syntax.OpAnyCharNotNL:
		b.WriteString(`[^\u000a]`)
	case syntax.OpAnyChar:
		// matches newlines too in PostgreSQL, unless newline-sensitive matching is turned on
		b.WriteByte('.')
	case syntax.OpBeginLine:
		b.WriteString(`(?:\A|(?<=\u000a))`)
	case syntax.OpEndLine:
		b.WriteString(`(?:\Z|(?=\u000a))`)
	case syntax.OpBeginText:
		b.WriteString(`\A`)
	case syntax.OpEndText:
		b.WriteString(`\Z`)
	case syntax.OpWordBoundary:
		// \y would use the locale's idea of a word character, but Go's is only ASCII
		b.WriteString(`(?:(?<!` + regexWord + `)(?=` + regexWord + `)|(?<=` + regexWord + `)(?!` + regexWord + `))`)
	case syntax.OpNoWordBoundary:
		b.WriteString(`(?:(?<=` + regexWord + `)(?=` + regexWord + `)|(?<!` + regexWord + `)(?!` + regexWord + `))`)
	case syntax.OpCapture:
		return writeRegexGroup(b, re.Sub[0], "")
	case syntax.OpStar:
		return writeRegexGroup(b, re.Sub[0], "*")
	case syntax.OpPlus:
		return writeRegexGroup(b, re.Sub[0], "+")
	case syntax.OpQuest:
		return writeRegexGroup(b, re.Sub[0], "?")
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if err := writeRegex(b, sub); err != nil {
				return err
			}
		}
	case syntax.OpAlternate:
		b.WriteString("(?:")
		for i, sub := range re.Sub {
			if i > 0 {
				b.WriteByte('|')
			}
			if err := writeRegex(b, sub); err != nil {
				return err
			}
		}
		b.WriteByte(')')
	default:
		return fmt.Errorf("unsupported regular expression operation %s", re.Op)
	}

	if b.Len() > maxTranslatedRegexLen {
		return errRegexTooComplex
	}
	return nil
}
