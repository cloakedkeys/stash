package jsonschema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review finding 1: a very long title still makes a file name that can be
// saved.
func TestStoryFilenameLongTitle(t *testing.T) {
	dir := t.TempDir()
	for _, title := range []string{
		strings.Repeat("A long title ", 60),
		strings.Repeat("Очень длинное название ", 40),
		strings.Repeat("とても長い題名", 120),
	} {
		fn := Story{}.Filename(title, "12345")
		assert.LessOrEqual(t, len(fn), 128, fn)
		assert.True(t, utf8.ValidString(fn), fn)
		assert.True(t, strings.HasSuffix(fn, ".12345.json"), fn)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fn), []byte("{}"), 0o644))
	}

	assert.Equal(t, "Short-title-", Story{}.Filename("Short title", "1")[:12])
}
