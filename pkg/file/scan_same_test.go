package file

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSameOnDisk(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{filepath.Join("One", "123"), filepath.Join("Two", "123")} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0755))
	}
	fsys := &OsFS{}

	assert.True(t, sameOnDisk(fsys, filepath.Join(root, "One", "123"), filepath.Join(root, "One", "123")))
	// the names that differ are two folders, so what is below them is too,
	// even with the same name
	assert.False(t, sameOnDisk(fsys, filepath.Join(root, "One", "123"), filepath.Join(root, "Two", "123")))
	assert.False(t, sameOnDisk(fsys, filepath.Join(root, "Gone", "123"), filepath.Join(root, "One", "123")))
	assert.False(t, sameOnDisk(fsys, filepath.Join(root, "One"), filepath.Join(root, "One", "123")))

	if ok, err := fsys.IsPathCaseSensitive(filepath.Join(root, "One")); err == nil && !ok {
		// a case-only rename of a folder above, as on Windows and macOS
		assert.True(t, sameOnDisk(fsys, filepath.Join(root, "One", "123"), filepath.Join(root, "one", "123")))
	}
}

// makeLink links name to the folder target. Windows only lets some users make
// links, but anyone can make a junction, which works the same way here.
func makeLink(t *testing.T, target, name string) {
	t.Helper()
	err := os.Symlink(target, name)
	if err != nil && runtime.GOOS == "windows" {
		err = exec.Command("cmd", "/c", "mklink", "/J", name, target).Run()
	}
	if err != nil {
		t.Skipf("can't make links here: %v", err)
	}
}

// Two links to one folder are two entries, and so is what is below them.
func TestSameOnDiskLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "123"), 0755))
	makeLink(t, target, filepath.Join(root, "First"))
	makeLink(t, target, filepath.Join(root, "Second"))
	fsys := &OsFS{}

	assert.False(t, sameOnDisk(fsys, filepath.Join(root, "First"), filepath.Join(root, "Second")))
	assert.False(t, sameOnDisk(fsys, filepath.Join(root, "First", "123"), filepath.Join(root, "Second", "123")))
}
