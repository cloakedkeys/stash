package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

type GalleryStore interface {
	SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error
	models.GalleryReaderWriter

	CountByImageID(ctx context.Context, imageID int) (int, error)
}
