package task

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// #1139: Clean Generated Files cleans image thumbnails and clip previews in
// the hash layout and the legacy folder.
func TestCleanGeneratedImageFiles(t *testing.T) {
	const (
		kept = "0123456789abcdef0123456789abcdef"
		gone = "0123fedcba9876543210fedcba987654" // same folders
	)

	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete", true: "dry run"}[dryRun], func(t *testing.T) {
			p := paths.NewPaths(t.TempDir(), "")
			files := func(c string) []string {
				var ret []string
				ret = append(ret, p.Generated.Thumbnail(c, models.DefaultGthumbWidth).Both()...)
				ret = append(ret, p.Generated.ClipPreview(c, models.DefaultGthumbWidth).Both()...)
				return ret
			}
			writeFiles(t, files(kept)...)
			writeFiles(t, files(gone)...)
			unknown := filepath.Join(p.Generated.Image, "01", "23", "notes.txt")
			writeFiles(t, unknown)

			db := mocks.NewDatabase()
			db.Image.On("FindByChecksum", mock.Anything, kept).Return([]*models.Image{{ID: 1}}, nil)
			db.Image.On("FindByChecksum", mock.Anything, gone).Return(nil, nil)

			j := CleanGeneratedJob{
				Options:                  CleanGeneratedOptions{ImageThumbnails: true, DryRun: dryRun},
				Paths:                    &p,
				Repository:               db.Repository(),
				BlobsStorageType:         config.BlobStorageTypeFilesystem,
				VideoFileNamingAlgorithm: models.HashAlgorithmOshash,
			}
			require.NoError(t, j.Execute(context.Background(), nil))

			for _, f := range files(kept) {
				require.FileExists(t, f, "an image that's still there")
			}
			for _, f := range files(gone) {
				if dryRun {
					require.FileExists(t, f)
				} else {
					require.NoFileExists(t, f, "a deleted image's file")
				}
			}
			require.FileExists(t, unknown, "unknown files are left alone")
		})
	}
}
