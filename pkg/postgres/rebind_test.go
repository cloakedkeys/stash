package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRebindPlaceholders(t *testing.T) {
	for query, want := range map[string]string{
		"SELECT 1":                         "SELECT 1",
		"a = ?":                            "a = $1",
		"a = ? AND b = ?":                  "a = $1 AND b = $2",
		"regex_match(x, '(?i)foo')":        "regex_match(x, '(?i)foo')",
		"regex_match(x, '(?i)foo', ?)":     "regex_match(x, '(?i)foo', $1)",
		"a = ? AND b = 'it''s?' AND c = ?": "a = $1 AND b = 'it''s?' AND c = $2",
		`"col?" = ?`:                       `"col?" = $1`,
		"a = ? -- why?\nAND b = ?":         "a = $1 -- why?\nAND b = $2",
		"a = ? /* why? */ AND b = ?":       "a = $1 /* why? */ AND b = $2",
		"a = $$x?$$ AND b = ?":             "a = $$x?$$ AND b = $1",
		"a = $t$x?$$?$t$ AND b = ?":        "a = $t$x?$$?$t$ AND b = $1",
		`a = E'it\'s?' AND b = ?`:          `a = E'it\'s?' AND b = $1`,
		"a = $1 AND b = '$'":               "a = $1 AND b = '$'",
		"x = 'open?":                       "x = 'open?",
		"a$b$ = ? AND c$d$ = ?":            "a$b$ = $1 AND c$d$ = $2",
	} {
		assert.Equal(t, want, rebindPlaceholders(query), query)
	}
}
