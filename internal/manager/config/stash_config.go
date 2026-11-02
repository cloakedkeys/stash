package config

import (
	"os"
	"path/filepath"

	"github.com/stashapp/stash/pkg/fsutil"
)

// Stash configuration details
type StashConfigInput struct {
	Path         string `json:"path"`
	ExcludeVideo bool   `json:"excludeVideo"`
	ExcludeAudio bool   `json:"excludeAudio"`
	ExcludeImage bool   `json:"excludeImage"`
	// nil when left out, as by a client from before stories; see KeepIncludeStory
	IncludeStory *bool `json:"includeStory"`
}

// KeepIncludeStory fills in IncludeStory where the input leaves it out: a library keeps
// its current value, and a new library leaves stories out. Otherwise saving the library
// list from an older client or script would switch stories off, and the next clean
// would remove them. A library is the same if its path names the same folder, so a
// trailing slash or different capital letters on a disk that ignores them don't make it
// a new library. A library with exactly the same path comes first.
func KeepIncludeStory(inputs []*StashConfigInput, current StashConfigs) {
	for _, in := range inputs {
		if in == nil || in.IncludeStory != nil {
			continue
		}
		v := false
		if s := current.find(in.Path, func(a, b string) bool { return a == b }); s != nil {
			v = s.IncludeStory
		} else if s := current.find(in.Path, sameFolder); s != nil {
			v = s.IncludeStory
		}
		in.IncludeStory = &v
	}
}

func (s StashConfigs) find(path string, same func(a, b string) bool) *StashConfig {
	for _, c := range s {
		if c != nil && same(c.Path, path) {
			return c
		}
	}
	return nil
}

// sameFolder reports whether a and b name the same folder: either the paths compare the
// same, the way the library lookups compare them, or both exist and are the same folder,
// such as on a Mac, whose disks usually ignore capital letters.
func sameFolder(a, b string) bool {
	if fsutil.IsPathInDir(a, b) && fsutil.IsPathInDir(b, a) {
		return true
	}
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

type StashConfig struct {
	Path         string `json:"path"`
	ExcludeVideo bool   `json:"excludeVideo"`
	ExcludeAudio bool   `json:"excludeAudio"`
	ExcludeImage bool   `json:"excludeImage"`
	// stories are left out unless a library includes them, since libraries often hold
	// text files, such as readmes, that aren't stories
	IncludeStory bool `json:"includeStory"`
}

type StashConfigs []*StashConfig

// GetStashFromPath returns the most specific stash configuration containing path.
func (s StashConfigs) GetStashFromPath(path string) *StashConfig {
	return s.GetStashFromDirPath(filepath.Dir(path))
}

// GetStashFromDirPath returns the most specific stash configuration containing dirPath.
func (s StashConfigs) GetStashFromDirPath(dirPath string) *StashConfig {
	var ret *StashConfig
	longestPath := -1

	for _, f := range s {
		if f == nil {
			continue
		}

		path := filepath.Clean(f.Path)
		if fsutil.IsPathInDir(path, dirPath) && len(path) > longestPath {
			ret = f
			longestPath = len(path)
		}
	}

	return ret
}

// GetStashRootFromDirPath returns the topmost configured stash path containing dirPath.
func (s StashConfigs) GetStashRootFromDirPath(dirPath string) string {
	var ret string
	shortestPath := -1

	for _, f := range s {
		if f == nil {
			continue
		}

		path := filepath.Clean(f.Path)
		if fsutil.IsPathInDir(path, dirPath) && (shortestPath == -1 || len(path) < shortestPath) {
			ret = path
			shortestPath = len(path)
		}
	}

	return ret
}

func (s StashConfigs) Paths() []string {
	paths := make([]string, len(s))
	for i, c := range s {
		// #6618 - clean the path to ensure comparison works correctly
		paths[i] = filepath.Clean(c.Path)
	}
	return paths
}
