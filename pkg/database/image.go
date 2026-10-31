package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

type ImageStore interface {
	SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error
	models.ImageReaderWriter

	OCountStore
}

type OCountStore interface {
	DecrementOCounter(ctx context.Context, id int) (int, error)
	IncrementOCounter(ctx context.Context, id int) (int, error)
	ResetOCounter(ctx context.Context, id int) (int, error)
}
