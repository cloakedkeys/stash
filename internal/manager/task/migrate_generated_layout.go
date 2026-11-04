package task

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models/paths"
)

// MigrateGeneratedLayoutJob moves generated files made before the hash layout
// from their legacy folders into it (#1139). Files are found where they are
// either way, so this is optional: it only tidies the generated folder.
//
// A file is moved only if its name is exactly where the paths package says
// that kind of file was kept, so anything else in the legacy folders, such as
// legacy scene screenshots, is left alone.
type MigrateGeneratedLayoutJob struct {
	Paths *paths.Paths

	moved, alreadyMoved, failed int
}

func (j *MigrateGeneratedLayoutJob) Execute(ctx context.Context, progress *job.Progress) error {
	gp := j.Paths.Generated
	steps := []struct {
		name string
		fn   func(ctx context.Context)
	}{
		{"video previews", func(ctx context.Context) { j.moveVideoFiles(ctx, gp.Screenshots) }},
		{"sprites", func(ctx context.Context) { j.moveVideoFiles(ctx, gp.Vtt) }},
		{"transcodes", func(ctx context.Context) { j.moveVideoFiles(ctx, gp.Transcodes) }},
		{"heatmaps", func(ctx context.Context) { j.moveVideoFiles(ctx, gp.InteractiveHeatmap) }},
		{"marker files", j.moveMarkerFiles},
		{"image thumbnails and clip previews", j.moveImageFiles},
	}

	progress.SetTotal(len(steps))
	for _, s := range steps {
		if job.IsCancelled(ctx) {
			logger.Info("Stopped moving generated files")
			return nil
		}
		progress.ExecuteTask("Moving "+s.name, func() { s.fn(ctx) })
		progress.Increment()
	}

	logger.Infof("Finished moving generated files to the new layout: %d moved, %d left because the new layout already had them, %d failed", j.moved, j.alreadyMoved, j.failed)
	return nil
}

// isGeneratedHash reports whether s can be a hash that generated files are
// named after: an oshash (16 hex digits) or an MD5 (32).
func isGeneratedHash(s string) bool {
	if len(s) != 16 && len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// readDir lists dir, which may not exist on a library started after the hash
// layout.
func readDir(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Errorf("Error reading %s: %v", dir, err)
		}
		return nil
	}
	return entries
}

// move moves path to the hash layout if it is the legacy location of one of
// candidates.
func (j *MigrateGeneratedLayoutJob) move(path string, candidates []paths.GeneratedFile) {
	for _, f := range candidates {
		if f.Legacy != path {
			continue
		}

		if exists, _ := fsutil.FileExists(f.Path); exists {
			// the new one is used; Clean Generated Files doesn't remove the old
			// one, because its scene or image is still there
			logger.Infof("Leaving %s: %s is already in the new layout", path, f.Path)
			j.alreadyMoved++
			return
		}

		if err := fsutil.EnsureDirAll(filepath.Dir(f.Path)); err != nil {
			logger.Errorf("Error creating folder for %s: %v", f.Path, err)
			j.failed++
			return
		}
		if err := os.Rename(path, f.Path); err != nil {
			logger.Errorf("Error moving %s to %s: %v", path, f.Path, err)
			j.failed++
			return
		}
		logger.Debugf("Moved %s to %s", path, f.Path)
		j.moved++
		return
	}
}

// moveVideoFiles moves the files in one of the flat legacy folders of video
// files, named <hash>.<ext> or <hash>_<kind>.
func (j *MigrateGeneratedLayoutJob) moveVideoFiles(ctx context.Context, dir string) {
	for _, e := range readDir(dir) {
		if ctx.Err() != nil {
			return
		}
		if e.IsDir() {
			continue
		}
		name := e.Name()
		hash := name
		if i := strings.IndexAny(name, "_."); i >= 0 {
			hash = name[:i]
		}
		if !isGeneratedHash(hash) {
			continue
		}
		j.move(filepath.Join(dir, name), j.Paths.Scene.GeneratedFiles(hash))
	}
}

// moveMarkerFiles moves the files in the legacy markers/<hash>/<seconds>.<ext>
// folders, and removes the folders left empty.
func (j *MigrateGeneratedLayoutJob) moveMarkerFiles(ctx context.Context) {
	root := j.Paths.Generated.Markers
	for _, d := range readDir(root) {
		if ctx.Err() != nil {
			return
		}
		hash := d.Name()
		if !d.IsDir() || !isGeneratedHash(hash) {
			continue
		}

		dir := filepath.Join(root, hash)
		for _, e := range readDir(dir) {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			seconds, err := strconv.Atoi(strings.TrimSuffix(name, filepath.Ext(name)))
			if err != nil {
				continue
			}
			j.move(filepath.Join(dir, name), j.Paths.SceneMarkers.MarkerFiles(hash, seconds))
		}
		removeIfEmpty(dir)
	}
}

// moveImageFiles moves the files in the legacy thumbnails/ab/cd folders, named
// <checksum>_<width>.<ext>, and removes the folders left empty.
func (j *MigrateGeneratedLayoutJob) moveImageFiles(ctx context.Context) {
	root := j.Paths.Generated.Thumbnails
	for _, d1 := range readDir(root) {
		if !d1.IsDir() {
			continue
		}
		dir1 := filepath.Join(root, d1.Name())
		for _, d2 := range readDir(dir1) {
			if !d2.IsDir() {
				continue
			}
			dir2 := filepath.Join(dir1, d2.Name())
			for _, e := range readDir(dir2) {
				if ctx.Err() != nil {
					return
				}
				if e.IsDir() {
					continue
				}
				name := e.Name()
				checksum, rest, ok := strings.Cut(name, "_")
				if !ok || !isGeneratedHash(checksum) {
					continue
				}
				width, err := strconv.Atoi(strings.TrimSuffix(rest, filepath.Ext(rest)))
				if err != nil {
					continue
				}
				j.move(filepath.Join(dir2, name), []paths.GeneratedFile{
					j.Paths.Generated.Thumbnail(checksum, width),
					j.Paths.Generated.ClipPreview(checksum, width),
				})
			}
			removeIfEmpty(dir2)
		}
		removeIfEmpty(dir1)
	}
}

func removeIfEmpty(dir string) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		if err := os.Remove(dir); err != nil {
			logger.Warnf("Error removing empty folder %s: %v", dir, err)
		}
	}
}
