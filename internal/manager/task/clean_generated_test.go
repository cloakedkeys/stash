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

func TestCleanGeneratedEmbeddedSubtitles(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "dry run"}[dryRun], func(t *testing.T) {
			p := paths.NewPaths(t.TempDir(), "")
			db := mocks.NewDatabase()
			db.Scene.On("Find", mock.Anything, 12).Return(&models.Scene{ID: 12}, nil).Once()
			db.Scene.On("Find", mock.Anything, 123).Return(nil, nil).Once()
			root := p.Generated.EmbeddedSubtitles
			for _, dir := range []string{"12", "123", "unknown/456", "123.delete", "12backup", "+123"} {
				name := filepath.Join(root, dir, "track.vtt")
				require.NoError(t, os.MkdirAll(filepath.Dir(name), 0755))
				require.NoError(t, os.WriteFile(name, []byte("WEBVTT"), 0600))
			}
			// Unknown empty folders and numeric files must also be left alone.
			require.NoError(t, os.MkdirAll(filepath.Join(root, "keep-empty"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "456"), []byte("keep"), 0600))
			j := CleanGeneratedJob{
				Options:                  CleanGeneratedOptions{DryRun: dryRun},
				Paths:                    &p,
				Repository:               db.Repository(),
				BlobsStorageType:         config.BlobStorageTypeFilesystem,
				VideoFileNamingAlgorithm: models.HashAlgorithmMd5,
			}
			require.NoError(t, j.Execute(context.Background(), nil))
			if dryRun {
				require.FileExists(t, filepath.Join(root, "123", "track.vtt"))
			} else {
				require.NoDirExists(t, filepath.Join(root, "123"))
			}
			for _, dir := range []string{"12", "unknown/456", "123.delete", "12backup", "+123"} {
				require.FileExists(t, filepath.Join(root, dir, "track.vtt"))
			}
			require.DirExists(t, filepath.Join(root, "keep-empty"))
			require.FileExists(t, filepath.Join(root, "456"))
			require.Equal(t, j.totalTasks, j.tasksComplete)
			db.Scene.AssertExpectations(t)
		})
	}
}

func TestCleanGeneratedWithoutEmbeddedSubtitleDirectory(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	j := CleanGeneratedJob{Paths: &p}
	require.NoError(t, j.cleanEmbeddedSubtitleFiles(context.Background()))
}
