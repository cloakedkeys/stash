package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
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
		{"CON", "_CON"},
		{"com1", "_com1"},
		{"LPT9.txt", "_LPT9.txt"},
		{"nul.tar.gz", "_nul.tar.gz"},
		{"Console", "Console"},
		{"Café crème", "Café crème"},
	} {
		got := storyFileName(c.title)
		assert.Equal(t, c.want, got, "title %q", c.title)
		assert.False(t, isReservedWindowsName(got+storyFileExt), "%q is a Windows device name", got+storyFileExt)
	}

	long := storyFileName(strings.Repeat("é", 300))
	assert.LessOrEqual(t, len(long), maxStoryNameBytes)
	assert.True(t, utf8.ValidString(long), "a long title isn't cut inside a character")
}

func TestWriteStoryText(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "downloaded")

	first, err := writeStoryText(dir, "Same name", "First text.", nil)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Same name.txt"), first, "the folder is made when it's missing")

	second, err := writeStoryText(dir, "Same name", "Second text.", nil)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Same name (2).txt"), second)

	// a name held by a missing file in the library is skipped, though it's free on disk
	missing := filepath.Join(dir, "Same name (3).txt")
	held := func(path string) (bool, error) { return path == missing, nil }
	third, err := writeStoryText(dir, "Same name", "Third text.", held)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Same name (4).txt"), third)
	assert.NoFileExists(t, missing)

	for path, want := range map[string]string{first: "First text.", second: "Second text.", third: "Third text."} {
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "an existing file is never replaced")
	}
}

func TestNameHeld(t *testing.T) {
	names := []string{"Été.txt", "Plain.txt", "ΣΟΦΊΑ.txt"}
	dir := t.TempDir()
	assert.True(t, nameHeld(names, filepath.Join(dir, "Été.txt")))
	assert.True(t, nameHeld(names, filepath.Join(dir, "plain.TXT")), "ASCII case")
	assert.True(t, nameHeld(names, filepath.Join(dir, "été.txt")), "case outside ASCII")
	assert.True(t, nameHeld(names, filepath.Join(dir, "σοφία.txt")), "Greek case")
	assert.False(t, nameHeld(names, filepath.Join(dir, "Ete.txt")), "accents still count")
	assert.False(t, nameHeld(nil, filepath.Join(dir, "Été.txt")))
}

// fakeFolders is a library's folders.
type fakeFolders []*models.Folder

func (ff fakeFolders) TopFolders(_ context.Context) ([]*models.Folder, error) {
	var ret []*models.Folder
	for _, f := range ff {
		if f.ParentFolderID == nil {
			ret = append(ret, f)
		}
	}
	return ret, nil
}

func (ff fakeFolders) FindByParentFolderID(_ context.Context, id models.FolderID) ([]*models.Folder, error) {
	var ret []*models.Folder
	for _, f := range ff {
		if f.ParentFolderID != nil && *f.ParentFolderID == id {
			ret = append(ret, f)
		}
	}
	return ret, nil
}

func TestFoldersAt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "lib")
	id := func(n int) *models.FolderID { v := models.FolderID(n); return &v }
	tree := fakeFolders{
		{ID: 1, Path: root},
		{ID: 2, Path: filepath.Join(root, "Été"), ParentFolderID: id(1)},
		{ID: 3, Path: filepath.Join(root, "été"), ParentFolderID: id(1)},
		{ID: 4, Path: filepath.Join(root, "Été", "Sub"), ParentFolderID: id(2)},
		{ID: 5, Path: filepath.Join(root, "Other"), ParentFolderID: id(1)},
		{ID: 6, Path: filepath.Join(t.TempDir(), "Été-lib")},
	}
	at := func(dir string) []models.FolderID {
		found, err := foldersAt(context.Background(), tree, dir)
		assert.NoError(t, err)
		var ret []models.FolderID
		for _, f := range found {
			ret = append(ret, f.ID)
		}
		return ret
	}

	assert.Equal(t, []models.FolderID{1}, at(root))
	assert.Equal(t, []models.FolderID{1}, at(root+string(filepath.Separator)), "a path ending in a separator")
	assert.Equal(t, []models.FolderID{1}, at(filepath.Join(filepath.Dir(root), "LIB")), "a library in another case")
	assert.Equal(t, []models.FolderID{2, 3}, at(filepath.Join(root, "ÉTÉ")), "case outside ASCII, and both folders differing only in case")
	assert.Equal(t, []models.FolderID{4}, at(filepath.Join(root, "été", "sub")), "a folder inside, such as a library inside another")
	assert.Equal(t, []models.FolderID{6}, at(filepath.Join(filepath.Dir(tree[5].Path), "été-LIB")), "a library in another case outside ASCII")
	assert.Empty(t, at(filepath.Join(root, "Ete")), "accents still count")
	assert.Empty(t, at(filepath.Join(root, "missing")))
	assert.Empty(t, at(filepath.Join(t.TempDir(), "elsewhere")), "a library that hasn't been scanned")
	assert.Empty(t, at(root+"x"), "a name that only starts like a library's")
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
