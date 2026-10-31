package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

type TagStore interface {
	SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error
	models.TagReaderWriter

	UpdateStashIDs(ctx context.Context, tagID int, stashIDs []models.StashID) error

	// blobJoinQueryBuilder
}
