package manager

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	fileimage "github.com/stashapp/stash/pkg/file/image"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type zipGIFFingerprints struct{}

func (zipGIFFingerprints) CalculateFingerprints(f *models.BaseFile, o file.Opener, useExisting bool) ([]models.Fingerprint, error) {
	return f.Fingerprints, nil
}

func TestScanZipGIFs(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 12, 8), color.Palette{color.Black, color.White})
	var animated, still bytes.Buffer
	require.NoError(t, gif.EncodeAll(&animated, &gif.GIF{
		Image: []*image.Paletted{frame, frame}, Delay: []int{20, 30},
	}))
	require.NoError(t, gif.Encode(&still, frame, nil))

	path := filepath.Join(t.TempDir(), "images.zip")
	out, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(out)
	// The broken GIF has a valid header: the old DecodeConfig-only scan
	// accepted it. Put it first to check that later entries are still scanned.
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"0-broken.gif", animated.Bytes()[:animated.Len()-3]},
		{"1-animated.GIF", animated.Bytes()},
		{"2-still.gif", still.Bytes()},
	} {
		w, err := zw.Create(entry.name)
		require.NoError(t, err)
		_, err = w.Write(entry.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, out.Close())
	info, err := os.Stat(path)
	require.NoError(t, err)
	zipFile := &models.BaseFile{ID: 10, Path: path, Basename: "images.zip", Size: info.Size()}
	zipID := zipFile.ID
	folder := &models.Folder{
		ID: 20, Path: path,
		DirEntry: models.DirEntry{ZipFileID: &zipID, ModTime: file.ModTime(info)},
	}
	db := mocks.NewDatabase()
	db.Folder.On("FindByPath", mock.Anything, path, true).Return(folder, nil)
	db.Folder.On("Update", mock.Anything, mock.Anything).Return(nil).Maybe()
	db.File.On("FindByPath", mock.Anything, mock.Anything, true).Return(nil, nil)
	var saved []models.File
	db.File.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		saved = append(saved, args.Get(1).(models.File))
	}).Return(nil)

	j := ScanJob{scanner: &file.Scanner{
		Repository:            file.NewRepository(db.Repository()),
		FingerprintCalculator: zipGIFFingerprints{},
		FileDecorators:        []file.Decorator{&fileimage.Decorator{}},
		RootPaths:             []string{path},
	}}
	err = j.scanZipFile(context.Background(), file.ScannedFile{
		BaseFile: zipFile, FS: &file.OsFS{}, Info: info,
	}, nil)
	require.NoError(t, err)
	require.Len(t, saved, 2, "corrupt GIF is skipped without stopping the ZIP scan")
	for i, f := range saved {
		vf, ok := f.(*models.VideoFile)
		require.True(t, ok, "%s scanned as %T", f.Base().Basename, f)
		require.Equal(t, "gif", vf.Format)
		require.Equal(t, "gif", vf.VideoCodec)
		require.Equal(t, 12, vf.Width)
		require.Equal(t, 8, vf.Height)
		require.Equal(t, &zipID, vf.ZipFileID)
		require.Equal(t, models.FolderID(20), vf.ParentFolderID)
		if i == 0 {
			require.Equal(t, 0.5, vf.Duration)
		} else {
			require.Equal(t, 0.1, vf.Duration)
		}
	}
	db.AssertExpectations(t)
}

func TestZipGIFClipPreviewSkipped(t *testing.T) {
	zipID := models.FileID(1)
	for _, overwrite := range []bool{false, true} {
		task := GenerateClipPreviewTask{
			Image: models.Image{Files: models.NewRelatedFiles([]models.File{
				&models.VideoFile{BaseFile: &models.BaseFile{
					DirEntry: models.DirEntry{ZipFileID: &zipID},
				}, Format: "gif"},
			})},
			Overwrite: overwrite,
		}
		// No manager or ffmpeg is configured: skipping must happen first.
		require.False(t, task.required())
		task.Start(context.Background())
	}
}
