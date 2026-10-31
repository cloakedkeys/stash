package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

type StudioStore interface {
	SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error
	models.StudioReaderWriter

	// blobJoinQueryBuilder
	tagRelationshipStore
}

type tagRelationshipStore interface {
	CountByTagID(ctx context.Context, tagID int) (int, error)
	GetTagIDs(ctx context.Context, id int) ([]int, error)
}
