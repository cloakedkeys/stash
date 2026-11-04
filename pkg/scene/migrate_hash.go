package scene

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models/paths"
)

// MigrateHash renames the generated files named after oldHash to newHash.
// Files in the hash layout are renamed within it, and files in a legacy
// location within that.
func MigrateHash(p *paths.Paths, oldHash string, newHash string) {
	scenePaths := p.Scene
	oldFiles := scenePaths.GeneratedFiles(oldHash)
	newFiles := scenePaths.GeneratedFiles(newHash)
	for i := range oldFiles {
		migrateSceneFiles(oldFiles[i].Path, newFiles[i].Path)
		migrateSceneFiles(oldFiles[i].Legacy, newFiles[i].Legacy)
	}

	// #2481: the vtt file names the sprite image
	oldSprite := filepath.Base(scenePaths.SpriteImage(oldHash).Path)
	newSprite := filepath.Base(scenePaths.SpriteImage(newHash).Path)
	for _, vtt := range scenePaths.SpriteVtt(newHash).Both() {
		migrateVttFile(vtt, oldSprite, newSprite)
	}

	// #3986 - migrate scene marker files
	markerPaths := p.SceneMarkers
	for _, oldPath := range markerPaths.ListMarkerFiles(oldHash) {
		migrateSceneFiles(oldPath, markerPaths.RenamedMarkerFile(oldPath, oldHash, newHash))
	}
	migrateSceneFolder(markerPaths.GetFolderPath(oldHash), markerPaths.GetFolderPath(newHash))
}

// InvalidateGeneratedFiles removes the generated files for hash. Used
// instead of MigrateHash when content changed at the same path - the old
// hash's files are stale, not valid to rename onto the new one.
func InvalidateGeneratedFiles(p *paths.Paths, hash string) {
	for _, f := range p.Scene.GeneratedFiles(hash) {
		for _, path := range f.Both() {
			removeSceneFile(path)
		}
	}

	for _, path := range p.SceneMarkers.ListMarkerFiles(hash) {
		removeSceneFile(path)
	}
	removeSceneFolder(p.SceneMarkers.GetFolderPath(hash))
}

// existsForAction reports whether path exists, ready to be renamed or
// removed, tolerating the check itself reporting IsNotExist as "doesn't
// exist" rather than an error.
func existsForAction(existsFn func(string) (bool, error), path string) bool {
	exists, err := existsFn(path)
	if err != nil && !os.IsNotExist(err) {
		logger.Errorf("Error checking existence of %s: %s", path, err.Error())
		return false
	}

	return exists
}

func removeSceneFile(path string) {
	removePath(fsutil.FileExists, os.Remove, "file", path)
}

func removeSceneFolder(path string) {
	removePath(fsutil.DirExists, fsutil.RemoveDir, "folder", path)
}

func removePath(existsFn func(string) (bool, error), removeFn func(string) error, kind, path string) {
	if !existsForAction(existsFn, path) {
		return
	}

	logger.Infof("removing outdated generated %s %s", kind, path)
	if err := removeFn(path); err != nil {
		logger.Errorf("error removing %s: %s", path, err.Error())
	}
}

func migrateSceneFiles(oldName, newName string) {
	if !existsForAction(fsutil.FileExists, oldName) {
		return
	}

	// in the hash layout, the new name may be in a folder that doesn't exist yet
	if err := fsutil.EnsureDirAll(filepath.Dir(newName)); err != nil {
		logger.Errorf("error creating folder for %s: %s", newName, err.Error())
		return
	}

	logger.Infof("renaming %s to %s", oldName, newName)
	if err := os.Rename(oldName, newName); err != nil {
		logger.Errorf("error renaming %s to %s: %s", oldName, newName, err.Error())
	}
}

// #2481: migrate vtt file contents in addition to renaming
func migrateVttFile(vttPath, oldSpriteBasename, newSpriteBasename string) {
	// #3356 - don't try to migrate if the file doesn't exist
	if !existsForAction(fsutil.FileExists, vttPath) {
		return
	}

	contents, err := os.ReadFile(vttPath)
	if err != nil {
		logger.Errorf("Error reading %s for vtt migration: %v", vttPath, err)
		return
	}

	contents = bytes.ReplaceAll(contents, []byte(oldSpriteBasename), []byte(newSpriteBasename))

	if err := os.WriteFile(vttPath, contents, 0644); err != nil {
		logger.Errorf("Error writing %s for vtt migration: %v", vttPath, err)
		return
	}
}

func migrateSceneFolder(oldName, newName string) {
	if !existsForAction(fsutil.DirExists, oldName) {
		return
	}

	logger.Infof("renaming %s to %s", oldName, newName)
	if err := os.Rename(oldName, newName); err != nil {
		logger.Errorf("error renaming %s to %s: %s", oldName, newName, err.Error())
	}
}
