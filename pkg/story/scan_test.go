package story

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestScanHandler(t *testing.T) {
	ctx := context.Background()
	f := &models.BaseFile{ID: 4, Path: "/stories/tale.txt"}

	t.Run("a new text file becomes a story", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Story.On("FindByFileID", ctx, f.ID).Return(nil, nil)
		db.Story.On("Create", ctx, mock.AnythingOfType("*models.Story"), []models.FileID{f.ID}).Return(nil)

		h := &ScanHandler{CreatorUpdater: db.Story}
		assert.NoError(t, h.Handle(ctx, f, nil))
		db.Story.AssertExpectations(t)
		created := db.Story.Calls[1].Arguments.Get(1).(*models.Story)
		assert.Empty(t, created.Title, "the title is left empty, as it is for a scanned scene")
	})

	t.Run("a file that has a story keeps it", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.Story.On("FindByFileID", ctx, f.ID).Return([]*models.Story{{ID: 1}}, nil)

		h := &ScanHandler{CreatorUpdater: db.Story}
		// a moved file is the same record, so its story is found the same way
		assert.NoError(t, h.Handle(ctx, f, &models.BaseFile{ID: 4, Path: "/old/tale.txt"}))
		db.Story.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything)
	})
}
