package postgres

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateRegex(t *testing.T) {
	for pattern, want := range map[string]string{
		"two words":   "two words",
		`\.mp4$`:      `\u002emp4\Z`,
		"MKV|MP4":     "M(?:KV|P4)",
		"(?i)a":       "[Aa]",
		"a.b":         `a[^\u000a]b`,
		"(?s)a.b":     "a.b",
		"(?P<y>a)":    "(?:a)",
		"a+?":         "(?:a)+",
		`\x{1F600}`:   `\U0001f600`,
		`\x00`:        `\Za`,
		"[\\x00]":     `\Za`,
		`[^\x00-\n]b`: `[\u000b-\U0010ffff]b`,
	} {
		got, err := translateRegex(pattern)
		require.NoError(t, err, pattern)
		assert.Equal(t, want, got, pattern)
	}
}

func TestTranslateRegexInvalid(t *testing.T) {
	for _, pattern := range []string{"*two words", "(", "a{1001}", `\p{NoSuchClass}`, `(?=a)`, "\xff"} {
		_, err := translateRegex(pattern)
		assert.Error(t, err, pattern)

		// the same patterns as Go's own regular expressions, and with the same message
		_, goErr := regexp.Compile(pattern)
		require.Error(t, goErr, pattern)
		assert.Equal(t, goErr.Error(), err.Error(), pattern)
	}
}

func TestTranslateRegexTooComplex(t *testing.T) {
	// valid in Go, but much too large once written out
	_, err := translateRegex(`\pL{1000}`)
	require.ErrorIs(t, err, errRegexTooComplex)
	assert.True(t, strings.Contains(err.Error(), `\pL{1000}`), "the error names the pattern")
}
