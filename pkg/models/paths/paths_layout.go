package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/pkg/fsutil"
)

// Generated files are grouped by the hash they're named after, two folder
// levels deep, so that no folder holds more than a few files:
// video/ab/cd/<hash>_<kind>.
const (
	generatedDirDepth  = 2
	generatedDirLength = 2
)

// GeneratedFile is a generated file, which files made before the hash layout
// may still have in its legacy location.
type GeneratedFile struct {
	// Path is where the file goes in the hash layout.
	Path string
	// Legacy is where the file was kept before the hash layout.
	Legacy string
}

// Find returns where the file is: in the hash layout if it's there, otherwise
// in its legacy location if it's there. If it's in neither, it returns where a
// new file goes.
func (f GeneratedFile) Find() string {
	if exists, _ := fsutil.FileExists(f.Path); exists {
		return f.Path
	}
	if f.Legacy != "" {
		if exists, _ := fsutil.FileExists(f.Legacy); exists {
			return f.Legacy
		}
	}
	return f.Path
}

// Both returns the file's path in both layouts.
func (f GeneratedFile) Both() []string {
	if f.Legacy == "" {
		return []string{f.Path}
	}
	return []string{f.Path, f.Legacy}
}

// hashDir returns the folder of the hash layout that files named after hash go in.
func hashDir(root, hash string) string {
	return filepath.Join(root, fsutil.GetIntraDir(hash, generatedDirDepth, generatedDirLength))
}

// videoFile returns the path of a video's generated file in the hash layout.
func (gp *generatedPaths) videoFile(hash, kind string) string {
	return filepath.Join(hashDir(gp.Video, hash), hash+"_"+kind)
}

// listVideoFiles returns the video's generated files in the hash layout whose
// kind starts with kindPrefix.
func (gp *generatedPaths) listVideoFiles(hash, kindPrefix string) []string {
	dir := hashDir(gp.Video, hash)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	prefix := hash + "_" + kindPrefix
	var ret []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			ret = append(ret, filepath.Join(dir, e.Name()))
		}
	}
	return ret
}
