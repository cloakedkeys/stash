package image

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func zipImageFile(id, zipID models.FileID) models.File {
	return &models.ImageFile{BaseFile: &models.BaseFile{ID: id, DirEntry: models.DirEntry{ZipFileID: &zipID}}}
}

// Two zips with the same pictures share one image per picture, with a file in each
// zip. Deleting one zip's gallery must keep the images, with the other zip's files.
func TestDestroyZipImagesKeepsImagesInAnotherZip(t *testing.T) {
	const (
		deletedZip models.FileID = 10
		keptZip    models.FileID = 20
		sharedID                 = 1
		ownID                    = 2
	)
	shared := &models.Image{ID: sharedID}
	own := &models.Image{ID: ownID}

	repo := &mocks.ImageReaderWriter{}
	repo.On("FindByZipFileID", mock.Anything, deletedZip).Return([]*models.Image{shared, own}, nil)
	repo.On("GetFiles", mock.Anything, sharedID).Return([]models.File{zipImageFile(11, deletedZip), zipImageFile(21, keptZip)}, nil)
	repo.On("GetFiles", mock.Anything, ownID).Return([]models.File{zipImageFile(12, deletedZip)}, nil)
	repo.On("RemoveFileID", mock.Anything, sharedID, models.FileID(11)).Return(nil)
	repo.On("Destroy", mock.Anything, ownID).Return(nil)

	s := &Service{Repository: repo}
	zip := &models.BaseFile{ID: deletedZip}
	destroyed, err := s.DestroyZipImages(context.Background(), zip, &FileDeleter{}, false)
	require.NoError(t, err)

	assert.Equal(t, []*models.Image{own}, destroyed, "only the image with no other file is destroyed")
	repo.AssertCalled(t, "RemoveFileID", mock.Anything, sharedID, models.FileID(11))
	repo.AssertNotCalled(t, "RemoveFileID", mock.Anything, sharedID, models.FileID(21))
	repo.AssertNotCalled(t, "Destroy", mock.Anything, sharedID)
	repo.AssertExpectations(t)
}
