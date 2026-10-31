package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

type FolderStore interface {
	models.FolderReaderWriter

	FindByIDs(ctx context.Context, ids []models.FolderID) ([]*models.Folder, error)
}
