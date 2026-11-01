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
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPrepareZipRewrites(t *testing.T) {
	for _, tc := range []struct {
		name             string
		fail             bool
		otherGalleryFile bool
		folderGallery    bool
		staleImages      bool
		deleteGenerated  bool
	}{
		{name: "destroys empty zip gallery"},
		{name: "rollback on gallery error", fail: true},
		{name: "keeps gallery with another file", otherGalleryFile: true},
		{name: "destroys stale images", staleImages: true},
		{name: "deletes stale image generated files", staleImages: true, deleteGenerated: true},
		{name: "restores stale image generated files on rollback", staleImages: true, deleteGenerated: true, fail: true},
		{name: "destroys folder galleries", folderGallery: true},
		{name: "keeps folder gallery with another file", folderGallery: true, otherGalleryFile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			images := &mocks.ImageReaderWriter{}
			files.On("Find", mock.Anything, models.FileID(1)).Return([]models.File{zf}, nil).Once()
			folderID := models.FolderID(2)
			folder := &models.Folder{ID: folderID}
			zipGallery := &models.Gallery{ID: 3}
			galleries.On("FindByFileID", mock.Anything, models.FileID(1)).Return([]*models.Gallery{zipGallery}, nil).Once()
			galleryFiles := []models.File{zf}
			if tc.otherGalleryFile && !tc.folderGallery {
				galleryFiles = append(galleryFiles, &models.BaseFile{ID: 10})
			}
			galleries.On("GetFiles", mock.Anything, 3).Return(galleryFiles, nil).Once()
			var folderGalleries []*models.Gallery
			if tc.folderGallery {
				folderGalleries = []*models.Gallery{{ID: 4, FolderID: &folderID}}
				folderFiles := []models.File{}
				if tc.otherGalleryFile {
					folderFiles = []models.File{&models.BaseFile{ID: 10}}
				} else {
					galleries.On("Destroy", mock.Anything, 4).Return(nil).Once()
				}
				galleries.On("GetFiles", mock.Anything, 4).Return(folderFiles, nil).Once()
			}
			galleries.On("FindByFolderID", mock.Anything, folderID).Return(folderGalleries, nil).Once()
			folderQueries := 2
			if tc.fail {
				folderQueries = 1
			}
			folders.On("FindByZipFileID", mock.Anything, models.FileID(1)).Return([]*models.Folder{folder}, nil).Times(folderQueries)

			p := paths.NewPaths(t.TempDir(), "")
			d := &image.FileDeleter{Deleter: file.NewDeleter(), Paths: &p}
			var zipImages []*models.Image
			var zipFiles []models.File
			var thumbPath string
			if tc.staleImages {
				missing := &models.BaseFile{ID: 5, DirEntry: models.DirEntry{ZipFileID: &zf.ID}, Path: filepath.Join(zipPath, "missing.jpg")}
				duplicate := &models.BaseFile{ID: 6, DirEntry: models.DirEntry{ZipFileID: &zf.ID}, Path: filepath.Join(zipPath, "duplicate.jpg")}
				zipFiles = []models.File{missing, duplicate}
				zipImages = []*models.Image{{ID: 7, Checksum: "stale"}, {ID: 8}}
				images.On("GetFiles", mock.Anything, 7).Return([]models.File{missing}, nil).Once()
				images.On("Destroy", mock.Anything, 7).Return(nil).Once()
				images.On("GetFiles", mock.Anything, 8).Return([]models.File{duplicate, &models.BaseFile{ID: 9}}, nil).Once()
				images.On("RemoveFileID", mock.Anything, 8, models.FileID(6)).Return(nil).Once()
				thumbPath = p.Generated.GetThumbnailPath("stale", models.DefaultGthumbWidth)
				require.NoError(t, os.MkdirAll(filepath.Dir(thumbPath), 0700))
				require.NoError(t, os.WriteFile(thumbPath, []byte("thumbnail"), 0600))
			}
			images.On("FindByZipFileID", mock.Anything, models.FileID(1)).Return(zipImages, nil).Once()
			if !tc.fail {
				files.On("FindByZipFileID", mock.Anything, models.FileID(1)).Return(zipFiles, nil).Once()
				for _, f := range zipFiles {
					files.On("Destroy", mock.Anything, f.Base().ID).Return(nil).Once()
				}
				folders.On("Destroy", mock.Anything, folderID).Return(nil).Once()
				files.On("Destroy", mock.Anything, models.FileID(1)).Return(nil).Once()
			}
			var destroyErr error
			if tc.fail {
				destroyErr = errors.New("gallery deletion failed")
			}
			if !tc.otherGalleryFile || tc.folderGallery {
				galleries.On("Destroy", mock.Anything, 3).Return(destroyErr).Once()
			}

			s := &Service{Repository: galleries, File: files, Folder: folders, ImageService: &image.Service{Repository: images, File: files}}
			d.ZipEntry(1, zipPath, "image.jpg")
			err = s.PrepareZipRewrites(context.Background(), d, tc.deleteGenerated)
			if tc.fail {
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
			if tc.staleImages {
				_, err := os.Stat(thumbPath)
				if tc.deleteGenerated && !tc.fail {
					require.True(t, os.IsNotExist(err), "thumbnail should be deleted: %v", err)
				} else {
					require.NoError(t, err)
				}
				images.AssertNotCalled(t, "Destroy", mock.Anything, 8)
			}
			if tc.otherGalleryFile {
				keptID := 3
				if tc.folderGallery {
					keptID = 4
				}
				galleries.AssertNotCalled(t, "Destroy", mock.Anything, keptID)
			}
			files.AssertExpectations(t)
			folders.AssertExpectations(t)
			galleries.AssertExpectations(t)
			images.AssertExpectations(t)
		})
	}
}
