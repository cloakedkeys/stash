package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

type GroupStore interface {
	SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error
	models.GroupReaderWriter

	FindInAncestors(ctx context.Context, ascestorIDs []int, ids []int) ([]int, error)
	FindSubGroupIDs(ctx context.Context, containingID int, ids []int) ([]int, error)

	blobJoinQueryBuilder
	tagRelationshipStore
	groupRelationshipStore
}

type groupRelationshipStore interface {
	AddSubGroups(ctx context.Context, groupID int, subGroups []models.GroupIDDescription, insertIndex *int) error
	GetContainingGroupDescriptions(ctx context.Context, id int) ([]models.GroupIDDescription, error)
	GetSubGroupDescriptions(ctx context.Context, id int) ([]models.GroupIDDescription, error)
	RemoveSubGroups(ctx context.Context, groupID int, subGroupIDs []int) error
	ReorderSubGroups(ctx context.Context, groupID int, subGroupIDs []int, insertPointID int, insertAfter bool) error
}
