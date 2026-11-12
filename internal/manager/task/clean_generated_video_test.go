package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func writeFiles(t *testing.T, files ...string) {
	t.Helper()
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	}
}

// #1139: clean generated files in the hash layout, by the options that clean
// them in their legacy folders.
func TestCleanGeneratedVideoFiles(t *testing.T) {
	const (
		kept = "0123456789abcdef" // a scene with a marker at 30 seconds
		gone = "0123fedcba987654" // no scene, in the same folder
	)

	files := func(p paths.Paths, hash string) map[string]string {
		return map[string]string{
			"sprite":    p.Scene.SpriteImage(hash).Path,
			"vtt":       p.Scene.SpriteVtt(hash).Path,
			"preview":   p.Scene.VideoPreview(hash).Path,
			"webp":      p.Scene.WebpPreview(hash).Path,
			"transcode": p.Scene.Transcode(hash).Path,
			"heatmap":   p.Scene.Heatmap(hash).Path,
			"marker30":  p.SceneMarkers.VideoPreview(hash, 30).Path,
			"marker60":  p.SceneMarkers.Screenshot(hash, 60).Path,
		}
	}

	for _, tc := range []struct {
		name    string
		options CleanGeneratedOptions
		cleaned []string // of the files of gone, and of kept's marker at 60
	}{
		{"everything", CleanGeneratedOptions{Sprites: true, Screenshots: true, Transcodes: true, Markers: true},
			[]string{"sprite", "vtt", "preview", "webp", "transcode", "marker30", "marker60"}},
		{"sprites only", CleanGeneratedOptions{Sprites: true}, []string{"sprite", "vtt"}},
		{"markers only", CleanGeneratedOptions{Markers: true}, []string{"marker30", "marker60"}},
		{"dry run", CleanGeneratedOptions{Sprites: true, Screenshots: true, Transcodes: true, Markers: true, DryRun: true}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := paths.NewPaths(t.TempDir(), "")
			keptFiles, goneFiles := files(p, kept), files(p, gone)
			for _, f := range []map[string]string{keptFiles, goneFiles} {
				for _, path := range f {
					writeFiles(t, path)
				}
			}
			unknown := filepath.Join(p.Generated.Video, "01", "23", "notes.txt")
			writeFiles(t, unknown)

			db := mocks.NewDatabase()
			oshash := func(h string) []models.Fingerprint {
				return []models.Fingerprint{{Type: models.FingerprintTypeOshash, Fingerprint: h}}
			}
			db.Scene.On("FindByFingerprints", mock.Anything, oshash(kept)).Return([]*models.Scene{{ID: 1, OSHash: kept}}, nil).Maybe()
			db.Scene.On("FindByFingerprints", mock.Anything, oshash(gone)).Return(nil, nil).Maybe()
			db.SceneMarker.On("FindBySceneID", mock.Anything, 1).Return([]*models.SceneMarker{{ID: 7, SceneID: 1, Seconds: 30}}, nil).Maybe()

			j := CleanGeneratedJob{
				Options:                  tc.options,
				Paths:                    &p,
				Repository:               db.Repository(),
				BlobsStorageType:         config.BlobStorageTypeFilesystem,
				VideoFileNamingAlgorithm: models.HashAlgorithmOshash,
			}
			require.NoError(t, j.Execute(context.Background(), nil))

			cleaned := map[string]bool{}
			for _, k := range tc.cleaned {
				cleaned[k] = true
			}
			for kind, path := range goneFiles {
				if cleaned[kind] {
					require.NoFileExists(t, path, "%s of a deleted scene", kind)
				} else {
					require.FileExists(t, path, "%s of a deleted scene", kind)
				}
			}
			for kind, path := range keptFiles {
				if kind == "marker60" && cleaned[kind] {
					require.NoFileExists(t, path, "a deleted marker's file")
				} else {
					require.FileExists(t, path, "%s of a scene that's still there", kind)
				}
			}
			require.FileExists(t, unknown, "unknown files are left alone")
			require.Equal(t, j.totalTasks, j.tasksComplete)
		})
	}
}

// Empty hash folders are removed once their files are cleaned.
func TestCleanGeneratedVideoFilesRemovesEmptyFolders(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	const gone = "abcdef0123456789"
	writeFiles(t, p.Scene.SpriteImage(gone).Path)

	db := mocks.NewDatabase()
	db.Scene.On("FindByFingerprints", mock.Anything, mock.Anything).Return(nil, nil)

	j := CleanGeneratedJob{
		Options:                  CleanGeneratedOptions{Sprites: true},
		Paths:                    &p,
		Repository:               db.Repository(),
		BlobsStorageType:         config.BlobStorageTypeFilesystem,
		VideoFileNamingAlgorithm: models.HashAlgorithmOshash,
	}
	require.NoError(t, j.Execute(context.Background(), nil))

	require.NoDirExists(t, filepath.Join(p.Generated.Video, "ab"))
	require.DirExists(t, p.Generated.Video)
}

// Files named after the other naming algorithm's hash aren't this algorithm's
// to judge, so they're left alone.
func TestCleanGeneratedVideoFilesKeepsOtherAlgorithm(t *testing.T) {
	const (
		oshash = "0123456789abcdef"
		md5    = "0123456789abcdef0123456789abcdef"
	)

	for _, algo := range []models.HashAlgorithm{models.HashAlgorithmOshash, models.HashAlgorithmMd5} {
		t.Run(algo.String(), func(t *testing.T) {
			p := paths.NewPaths(t.TempDir(), "")
			other := p.Scene.SpriteImage(md5).Path
			if algo == models.HashAlgorithmMd5 {
				other = p.Scene.SpriteImage(oshash).Path
			}
			writeFiles(t, other)

			// no scene has any hash
			db := mocks.NewDatabase()
			db.Scene.On("FindByFingerprints", mock.Anything, mock.Anything).Return(nil, nil).Maybe()

			j := CleanGeneratedJob{
				Options:                  CleanGeneratedOptions{Sprites: true},
				Paths:                    &p,
				Repository:               db.Repository(),
				BlobsStorageType:         config.BlobStorageTypeFilesystem,
				VideoFileNamingAlgorithm: algo,
			}
			require.NoError(t, j.Execute(context.Background(), nil))

			require.FileExists(t, other)
			db.Scene.AssertNotCalled(t, "FindByFingerprints", mock.Anything, mock.Anything)
		})
	}
}

func TestCleanGeneratedPartKeys(t *testing.T) {
	for _, algo := range []models.HashAlgorithm{models.HashAlgorithmOshash, models.HashAlgorithmMd5} {
		t.Run(algo.String(), func(t *testing.T) {
			hash := "0123456789abcdef"
			if algo == models.HashAlgorithmMd5 {
				hash += hash
			}
			p := paths.NewPaths(t.TempDir(), "")
			start, end := 10.0, 20.0
			bounded := &models.Scene{ID: 1, OSHash: hash, Checksum: hash, StartTime: &start, EndTime: &end}
			open := *bounded
			open.ID = 2
			open.EndTime = nil
			equal := *bounded
			equal.ID = 3
			duration := *bounded
			duration.ID = 4
			duration.EndTime = nil
			duration.PrimaryFileDuration = 60
			valid := []string{hash + "_10000-20000", hash + "_10000-end", hash + "_10000-60000"}
			orphan := []string{hash + "_10000-30000", hash + "_30000-end", hash}
			var kept, gone []string
			for _, group := range []struct {
				keys   []string
				exists bool
			}{{valid, true}, {orphan, false}} {
				for _, key := range group.keys {
					files := []string{p.Scene.SpriteImage(key).Path, p.Scene.SpriteImage(key).Legacy, p.Scene.SpriteVtt(key).Legacy, p.Scene.VideoPreview(key).Path, p.Scene.VideoPreview(key).Legacy, p.Scene.Transcode(key).Path, p.Scene.Transcode(key).Legacy, p.SceneMarkers.VideoPreview(key, 30).Path, p.SceneMarkers.VideoPreview(key, 30).Legacy}
					writeFiles(t, files...)
					if group.exists {
						kept = append(kept, files...)
					} else {
						gone = append(gone, files...)
					}
				}
			}
			// Markers on equal-key scenes keep files; markers on other ranges cannot.
			wrongMarker := p.SceneMarkers.VideoPreview(valid[1], 40).Path
			writeFiles(t, wrongMarker)
			gone = append(gone, wrongMarker)
			db := mocks.NewDatabase()
			db.Scene.On("FindByFingerprints", mock.Anything, mock.Anything).Return([]*models.Scene{bounded, &open, &equal, &duration}, nil)
			db.SceneMarker.On("FindBySceneID", mock.Anything, 1).Return([]*models.SceneMarker{{Seconds: 40}}, nil)
			for _, id := range []int{2, 3, 4} {
				db.SceneMarker.On("FindBySceneID", mock.Anything, id).Return([]*models.SceneMarker{{Seconds: 30}}, nil)
			}
			j := CleanGeneratedJob{Options: CleanGeneratedOptions{Sprites: true, Screenshots: true, Transcodes: true, Markers: true}, Paths: &p, Repository: db.Repository(), BlobsStorageType: config.BlobStorageTypeFilesystem, VideoFileNamingAlgorithm: algo}
			require.NoError(t, j.Execute(context.Background(), nil))
			for _, f := range kept {
				require.FileExists(t, f)
			}
			for _, f := range gone {
				require.NoFileExists(t, f)
			}
		})
	}
}
