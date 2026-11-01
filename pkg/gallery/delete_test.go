package gallery

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPrepareZipRewritesDestroysEmptyZipGallery(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback on gallery error"}[fail], func(t *testing.T) {
			var archive bytes.Buffer
			w := zip.NewWriter(&archive)
			entry, err := w.Create("image.jpg")
			require.NoError(t, err)
			_, err = entry.Write([]byte("image"))
			require.NoError(t, err)
			require.NoError(t, w.Close())
			zipPath := filepath.Join(t.TempDir(), "gallery.zip")
			require.NoError(t, os.WriteFile(zipPath, archive.Bytes(), 0600))
			zf := &models.BaseFile{ID: 1, Path: zipPath}

			files := &mocks.FileReaderWriter{}
			folders := &mocks.FolderReaderWriter{}
			galleries := &mocks.GalleryReaderWriter{}
			files.On("Find", mock.Anything, models.FileID(1)).Return([]models.File{zf}, nil).Once()
			files.On("FindByZipFileID", mock.Anything, models.FileID(1)).Return([]models.File{}, nil).Once()
			folders.On("FindByZipFileID", mock.Anything, models.FileID(1)).Return([]*models.Folder{{ID: 2}}, nil).Once()
			folders.On("Destroy", mock.Anything, models.FolderID(2)).Return(nil).Once()
			files.On("Destroy", mock.Anything, models.FileID(1)).Return(nil).Once()
			galleries.On("FindByFileID", mock.Anything, models.FileID(1)).Return([]*models.Gallery{{ID: 3}}, nil).Once()
			var destroyErr error
			if fail {
				destroyErr = errors.New("gallery deletion failed")
			}
			galleries.On("Destroy", mock.Anything, 3).Return(destroyErr).Once()

			s := &Service{Repository: galleries, File: files, Folder: folders}
			d := &image.FileDeleter{Deleter: file.NewDeleter()}
			d.ZipEntry(1, zipPath, "image.jpg")
			err = s.PrepareZipRewrites(context.Background(), d)
			if fail {
				require.ErrorIs(t, err, destroyErr)
				d.Rollback()
				got, err := os.ReadFile(zipPath)
				require.NoError(t, err)
				require.Equal(t, archive.Bytes(), got)
			} else {
				require.NoError(t, err)
				d.Commit()
				_, err := os.Stat(zipPath)
				require.True(t, os.IsNotExist(err), "zip should be deleted: %v", err)
			}
			files.AssertExpectations(t)
			folders.AssertExpectations(t)
			galleries.AssertExpectations(t)
		})
	}
}
