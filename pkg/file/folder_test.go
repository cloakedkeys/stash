package file

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTransferZipHierarchyAcrossPlatforms(t *testing.T) {
	for _, tc := range []struct {
		name, dir, zipFolder, nestedFolder string
	}{
		{"windows", `D:\stash`, `D:\stash\gallery.zip`, `D:\stash\gallery.zip\chapter\nested`},
		{"windows slashes", "D:/stash", "D:/stash/gallery.zip", "D:/stash/gallery.zip/chapter/nested"},
		{"windows UNC", `\\server\stash`, `\\server\stash\gallery.zip`, `\\server\stash\gallery.zip\chapter\nested`},
		{"unix", "/mnt/stash", "/mnt/stash/gallery.zip", "/mnt/stash/gallery.zip/chapter/nested"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := mocks.NewDatabase()
			zipID := models.FileID(1)
			newPath := filepath.Join(t.TempDir(), "gallery.zip")
			oldFolders := []*models.Folder{
				{ID: 10, Path: tc.zipFolder, DirEntry: models.DirEntry{ZipFileID: &zipID}},
				{ID: 11, Path: tc.nestedFolder, DirEntry: models.DirEntry{ZipFileID: &zipID}},
			}
			newFolders := []*models.Folder{
				{ID: 20, Path: newPath},
				{ID: 21, Path: filepath.Join(newPath, "chapter", "nested")},
			}
			var entries []models.File
			for i, folder := range oldFolders {
				// Both database backends join stored folder paths with basenames
				// using the current OS, even when the folder came from another OS.
				entry := &models.BaseFile{
					ID: models.FileID(30 + i), Path: filepath.Join(folder.Path, "0001.jpg"),
					Basename: "0001.jpg", ParentFolderID: folder.ID, DirEntry: models.DirEntry{ZipFileID: &zipID},
				}
				entries = append(entries, entry)
				newFolder := newFolders[i]
				db.Folder.On("FindByPath", mock.Anything, newFolder.Path, true).Return(newFolder, nil)
				db.Folder.On("Update", mock.Anything, newFolder).Return(nil).Once()
				db.Folder.On("Update", mock.Anything, folder).Return(nil).Once()
				db.File.On("Update", mock.Anything, entry).Return(nil).Once()
			}
			db.Folder.On("FindByZipFileID", mock.Anything, zipID).Return(oldFolders, nil).Once()
			db.File.On("FindByZipFileID", mock.Anything, zipID).Return(entries, nil).Once()

			mover := zipHierarchyMover{folderStore: db.Folder, files: db.File}
			err := mover.transferZipHierarchy(ctx, zipID, filepath.Join(tc.dir, "gallery.zip"), newPath)
			require.NoError(t, err)
			for i, entry := range entries {
				assert.Equal(t, newFolders[i].ID, entry.Base().ParentFolderID)
				assert.Equal(t, &zipID, entry.Base().ZipFileID)
				assert.Equal(t, &zipID, newFolders[i].ZipFileID)
				assert.Nil(t, oldFolders[i].ZipFileID)
			}
			db.File.AssertExpectations(t)
			db.Folder.AssertExpectations(t)
		})
	}
}

func TestZipRelativePath(t *testing.T) {
	for _, tc := range []struct {
		name, zipPath, entryPath, relative string
		ok                                 bool
	}{
		{"root", `D:\stash/gallery.zip`, `D:\stash\gallery.zip`, ".", true},
		{"mixed separators", `D:\stash/gallery.zip`, `D:\stash\gallery.zip\chapter/0001.jpg`, "chapter/0001.jpg", true},
		{"UNC", `\\server\stash/gallery.zip`, `\\server\stash\gallery.zip\chapter/0001.jpg`, "chapter/0001.jpg", true},
		{"unix", "/mnt/stash/gallery.zip", "/mnt/stash/gallery.zip/chapter/0001.jpg", "chapter/0001.jpg", true},
		{"different archive", "/mnt/stash/gallery.zip", "/mnt/stash/other.zip/0001.jpg", "", false},
		{"shared prefix", "/mnt/stash/gallery.zip", "/mnt/stash/gallery.zip.bak/0001.jpg", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok := zipRelativePath(tc.zipPath, tc.entryPath)
			assert.Equal(t, tc.ok, ok)
			if ok {
				assert.Equal(t, filepath.FromSlash(tc.relative), rel)
			}
		})
	}
	if filepath.Separator == '/' {
		// Backslashes in Unix names are literal, not Windows separators.
		rel, ok := zipRelativePath(`/mnt/stash/gallery.zip`, `/mnt/stash/gallery.zip/chapter\name/0001.jpg`)
		assert.True(t, ok)
		assert.Equal(t, `chapter\name/0001.jpg`, rel)
	}
}
