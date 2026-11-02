package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoryFileName(t *testing.T) {
	for _, c := range []struct{ title, want string }{
		{"Same name", "Same name"},
		{"Part 1: The Start", "Part 1_ The Start"},
		{`a/b\c<d>e"f|g?h*i`, "a_b_c_d_e_f_g_h_i"},
		{"Tabs\tand\nnew lines", "Tabs_and_new lines"},
		{"  spaced   out  ", "spaced out"},
		{"Ends with dots...", "Ends with dots"},
		{"..", "Story"},
		{"", "Story"},
		{"   ", "Story"},
		{"CON", "CON_"},
		{"com1", "com1_"},
		{"LPT9.txt", "LPT9.txt_"},
		{"Console", "Console"},
		{"Café crème", "Café crème"},
	} {
		assert.Equal(t, c.want, storyFileName(c.title), "title %q", c.title)
	}

	long := storyFileName(strings.Repeat("é", 300))
	assert.LessOrEqual(t, len(long), maxStoryNameBytes)
	assert.True(t, utf8.ValidString(long), "a long title isn't cut inside a character")
}

func TestWriteStoryText(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "downloaded")

	first, err := writeStoryText(dir, "Same name", "First text.")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Same name.txt"), first, "the folder is made when it's missing")

	second, err := writeStoryText(dir, "Same name", "Second text.")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Same name (2).txt"), second)

	third, err := writeStoryText(dir, "Same name", "Third text.")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Same name (3).txt"), third)

	for path, want := range map[string]string{first: "First text.", second: "Second text.", third: "Third text."} {
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "an existing file is never replaced")
	}
}

func TestCheckStoryDownloadPath(t *testing.T) {
	root := t.TempDir()
	stories := filepath.Join(root, "stories")
	videos := filepath.Join(root, "videos")

	cfg := config.InitializeEmpty()
	cfg.SetInterface(config.Stash, []*config.StashConfig{
		{Path: stories, IncludeStory: true},
		{Path: videos},
	})

	assert.NoError(t, CheckStoryDownloadPath(cfg, stories))
	assert.NoError(t, CheckStoryDownloadPath(cfg, filepath.Join(stories, "downloaded")))
	assert.Error(t, CheckStoryDownloadPath(cfg, videos), "a library without stories")
	assert.Error(t, CheckStoryDownloadPath(cfg, filepath.Join(root, "elsewhere")), "outside every library")

	cfg.SetInterface(config.StoryExtensions, []string{"md"})
	assert.Error(t, CheckStoryDownloadPath(cfg, stories), "txt isn't a story extension")
}
