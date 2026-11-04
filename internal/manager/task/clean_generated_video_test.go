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
			db.Scene.On("FindByFingerprints", mock.Anything, oshash(kept)).Return([]*models.Scene{{ID: 1}}, nil).Maybe()
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
