package task

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// Videos' generated files in the hash layout are named <hash>_<kind>. Each
// kind is cleaned by the option that cleans it in its legacy folder.
const (
	videoKindMarkerPrefix = "marker_"
)

func (j *CleanGeneratedJob) cleansVideoFiles() bool {
	o := j.Options
	return o.Sprites || o.Screenshots || o.Transcodes || o.Markers
}

// cleansVideoKind reports whether the options clean generated video files of kind.
func (j *CleanGeneratedJob) cleansVideoKind(kind string) bool {
	switch {
	case kind == "sprite.jpg" || kind == "thumbs.vtt":
		return j.Options.Sprites
	case kind == "preview.mp4" || kind == "preview.webp":
		return j.Options.Screenshots
	case kind == "transcode.mp4":
		return j.Options.Transcodes
	case strings.HasPrefix(kind, videoKindMarkerPrefix):
		return j.Options.Markers
	}

	// heatmaps aren't cleaned, as in their legacy folder
	return false
}

// parseVideoFileName splits a generated video file's name into the hash of the
// current naming algorithm and its kind.
func (j *CleanGeneratedJob) parseVideoFileName(name string) (hash, kind string, ok bool) {
	hash, kind, ok = strings.Cut(name, "_")
	if !ok {
		return "", "", false
	}
	if first, rest, hasRange := strings.Cut(kind, "_"); hasRange && strings.Contains(first, "-") {
		hash += "_" + first
		kind = rest
	}
	key, err := j.generatedFileKey(hash)
	if err != nil || kind == "" {
		return "", "", false
	}
	return key, kind, true
}

// markerKindSeconds returns the seconds of a marker file's kind, marker_<seconds>.<ext>.
func markerKindSeconds(kind string) (int, error) {
	var seconds int
	var ext string
	if _, err := fmt.Sscanf(strings.TrimPrefix(kind, videoKindMarkerPrefix), "%d.%s", &seconds, &ext); err != nil {
		return 0, err
	}
	return seconds, nil
}

// cleanVideoFiles deletes the generated video files in the hash layout of
// scenes, and markers, that no longer exist.
func (j *CleanGeneratedJob) cleanVideoFiles(ctx context.Context, progress *job.Progress) error {
	if job.IsCancelled(ctx) {
		return nil
	}

	root := j.Paths.Generated.Video
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}

	logger.Infof("Cleaning video files")

	// the files of a hash are next to each other in the walk, so look each
	// hash up once
	var (
		lastHash string
		found    bool
		scenes   []*models.Scene
		markers  []*models.SceneMarker
	)

	lookup := func(hash string) {
		lastHash = hash
		found = false
		scenes, markers = nil, nil

		if err := j.Repository.WithReadTxn(ctx, func(ctx context.Context) error {
			var err error
			scenes, err = j.getScenesWithHash(ctx, hash)
			if err != nil {
				return err
			}

			if j.Options.Markers {
				for _, s := range scenes {
					m, err := j.Repository.SceneMarker.FindBySceneID(ctx, s.ID)
					if err != nil {
						return err
					}
					markers = append(markers, m...)
				}
			}
			return nil
		}); err != nil {
			logger.Errorf("error checking scene entry for %s: %v", hash, err)
			return
		}

		found = true
	}

	if err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if err = ctx.Err(); err != nil {
			return err
		}

		if info.IsDir() {
			if path == root {
				return nil
			}

			// two levels of two character hash prefixes
			rel, _ := filepath.Rel(root, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if _, err := j.getIntraFolderPrefix(info.Name()); err != nil || len(info.Name()) != 2 || depth > 2 {
				logger.Warnf("Ignoring unknown directory: %s", path)
				return fs.SkipDir
			}

			if depth == 1 {
				j.setProgressFromFilename(info.Name(), progress)
			}

			return nil
		}

		filename := info.Name()
		hash, kind, ok := j.parseVideoFileName(filename)
		if !ok {
			logger.Warnf("Ignoring unknown video file: %s", filename)
			return nil
		}

		if !j.cleansVideoKind(kind) {
			return nil
		}

		if hash != lastHash {
			lookup(hash)
		}

		if !found {
			// couldn't check, so keep it
			return nil
		}

		if len(scenes) == 0 {
			j.logDelete("deleting unused video file: %s", filename)
			j.deleteFile(path)
			return nil
		}

		if strings.HasPrefix(kind, videoKindMarkerPrefix) {
			seconds, err := markerKindSeconds(kind)
			if err != nil {
				logger.Warnf("Ignoring unknown marker file: %s", filename)
				return nil
			}

			for _, m := range markers {
				if int(m.Seconds) == seconds {
					return nil
				}
			}

			j.logDelete("deleting unused marker file: %s", filename)
			j.deleteFile(path)
		}

		return nil
	}); err != nil {
		return err
	}

	j.removeEmptyHashDirs(root)

	return nil
}

// removeEmptyHashDirs removes the empty folders of the two folder levels of a
// hash layout.
func (j *CleanGeneratedJob) removeEmptyHashDirs(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			j.removeEmptyDirs(filepath.Join(root, entry.Name()))
		}
	}

	j.removeEmptyDirs(root)
}
