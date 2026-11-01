package postgres

import (
	"regexp"
	"strconv"
	"strings"
)

var dollarQuoteTag = regexp.MustCompile(`^\$(?:[A-Za-z_\x{80}-\x{10FFFF}][A-Za-z0-9_\x{80}-\x{10FFFF}]*)?\$`)

// rebindPlaceholders turns ? placeholders into PostgreSQL's $1, $2 and so on. A ? inside a
// string, quoted name or comment isn't a placeholder, and is left alone. Raw SQL from plugins
// can have them, such as a regular expression in a string.
func rebindPlaceholders(query string) string {
	if !strings.Contains(query, "?") {
		return query
	}

	var b strings.Builder
	n := 0
	for i := 0; i < len(query); {
		end := i + 1
		switch c := query[i]; {
		case c == '?':
			n++
			b.WriteString("$" + strconv.Itoa(n))
			i++
			continue
		case c == '\'':
			// an E straight before the quote, unless it ends a word such as LIKE
			backslashEscapes := i > 0 && (query[i-1] == 'E' || query[i-1] == 'e') && (i == 1 || !isIdentifierChar(query[i-2]))
			end = quotedEnd(query, i, c, backslashEscapes)
		case c == '"':
			end = quotedEnd(query, i, c, false)
		case strings.HasPrefix(query[i:], "--"):
			end = endAfter(query, i, "\n")
		case strings.HasPrefix(query[i:], "/*"):
			end = commentEnd(query, i)
		case c == '$' && (i == 0 || !isIdentifierChar(query[i-1])):
			if tag := dollarQuoteTag.FindString(query[i:]); tag != "" {
				end = endAfter(query, i+len(tag), tag)
			}
		}
		b.WriteString(query[i:end])
		i = end
	}
	return b.String()
}

// isIdentifierChar reports whether c can be part of a name: a $ after one, as in a$b$, doesn't
// start a dollar-quoted string.
func isIdentifierChar(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// quotedEnd returns the position after the closing quote. A doubled quote doesn't close it, since
// the second one opens straight away. An unclosed quote runs to the end of the query.
func quotedEnd(query string, start int, quote byte, backslashEscapes bool) int {
	for i := start + 1; i < len(query); i++ {
		switch query[i] {
		case '\\':
			if backslashEscapes {
				i++
			}
		case quote:
			return i + 1
		}
	}
	return len(query)
}

// commentEnd returns the position after a block comment starting at start. They can be nested.
func commentEnd(query string, start int) int {
	depth := 0
	for i := start; i < len(query)-1; i++ {
		switch query[i : i+2] {
		case "/*":
			depth++
			i++
		case "*/":
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(query)
}

// endAfter returns the position after the next occurrence of marker, or the end of the query.
func endAfter(query string, start int, marker string) int {
	if i := strings.Index(query[start:], marker); i >= 0 {
		return start + i + len(marker)
	}
	return len(query)
}

// temporary
