package file

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
)

// A file that fails to delete must not be returned again, otherwise the purge never finishes.
func TestPurgeMissingFinishesWhenDeleteFails(t *testing.T) {
	db := mocks.NewDatabase()

	missing := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	newFile := func(id models.FileID, path string) models.File {
		return &models.BaseFile{
			ID:       id,
			Path:     path,
			DirEntry: models.DirEntry{MissingSince: &missing},
		}
	}
	// a full batch, so that more results are looked for afterwards; the first file fails to delete
	var files []models.File
	for i := 1; i <= purgeBatchSize; i++ {
		files = append(files, newFile(models.FileID(i), fmt.Sprintf("/stash/%d.mp4", i)))
	}
	folder := &models.Folder{ID: 1, Path: "/stash/gone", DirEntry: models.DirEntry{MissingSince: &missing}}

	db.File.On("CountMissingInPaths", mock.Anything, mock.Anything, mock.Anything).Return(len(files), nil).Once()
	db.Folder.On("CountMissingInPaths", mock.Anything, mock.Anything, mock.Anything).Return(1, nil).Once()

	// each page may only be queried once; querying the first page again after a failed delete
	// is what used to loop forever
	db.File.On("FindMissingInPaths", mock.Anything, mock.Anything, mock.Anything, purgeBatchSize, 0).
		Return(files, nil).Once()
	db.File.On("FindMissingInPaths", mock.Anything, mock.Anything, mock.Anything, purgeBatchSize, purgeBatchSize).
		Return([]models.File{}, nil).Once()
	db.Folder.On("FindMissingInPaths", mock.Anything, mock.Anything, mock.Anything, purgeBatchSize, 0).
		Return([]*models.Folder{folder}, nil).Once()

	db.File.On("Destroy", mock.Anything, models.FileID(1)).Return(errors.New("locked")).Once()
	db.File.On("Destroy", mock.Anything, mock.MatchedBy(func(id models.FileID) bool { return id != 1 })).
		Return(nil).Times(purgeBatchSize - 1)
	db.Folder.On("Destroy", mock.Anything, models.FolderID(1)).Return(errors.New("locked")).Once()

	purger := &MissingPurger{
		Repository: Repository{
			TxnManager: db,
			File:       db.File,
			Folder:     db.Folder,
		},
	}

	done := make(chan struct{})
	m := job.NewManager()
	m.Start(context.Background(), "purge", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		defer close(done)
		purger.PurgeMissing(ctx, PurgeMissingOptions{}, progress)
		return nil
	}))

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("purge did not finish")
	}

	db.AssertExpectations(t)
}
