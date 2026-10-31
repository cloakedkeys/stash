package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

type PerformerStore interface {
	SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error
	models.PerformerReaderWriter

	DestroyImage(ctx context.Context, id int, blobCol string) error
	// blobJoinQueryBuilder
	customFieldsStore
}
