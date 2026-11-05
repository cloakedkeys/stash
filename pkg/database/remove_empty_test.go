//go:build integration
// +build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// Removing nothing does nothing, on both databases. On PostgreSQL an empty list used to become
// `IN ()`, a syntax error that also broke the rest of the transaction (#2201).
func TestRemoveNothing(t *testing.T) {
	removes := map[string]func(ctx context.Context, ids []int) error{
		"scene tags": func(ctx context.Context, ids []int) error {
			_, err := db.Scene().UpdatePartial(ctx, sceneIDs[sceneIdxWithTwoTags], models.ScenePartial{TagIDs: &models.UpdateIDs{IDs: ids, Mode: models.RelationshipUpdateModeRemove}})
			return err
		},
		"scene performers": func(ctx context.Context, ids []int) error {
			_, err := db.Scene().UpdatePartial(ctx, sceneIDs[sceneIdxWithTwoTags], models.ScenePartial{PerformerIDs: &models.UpdateIDs{IDs: ids, Mode: models.RelationshipUpdateModeRemove}})
			return err
		},
		"group sub groups": func(ctx context.Context, ids []int) error {
			groups := make([]models.GroupIDDescription, len(ids))
			for i, id := range ids {
				groups[i] = models.GroupIDDescription{GroupID: id}
			}
			_, err := db.Group().UpdatePartial(ctx, groupIDs[groupIdxWithChild], models.GroupPartial{SubGroups: &models.UpdateGroupDescriptions{Groups: groups, Mode: models.RelationshipUpdateModeRemove}})
			return err
		},
		"group containing groups": func(ctx context.Context, ids []int) error {
			groups := make([]models.GroupIDDescription, len(ids))
			for i, id := range ids {
				groups[i] = models.GroupIDDescription{GroupID: id}
			}
			_, err := db.Group().UpdatePartial(ctx, groupIDs[groupIdxWithParent], models.GroupPartial{ContainingGroups: &models.UpdateGroupDescriptions{Groups: groups, Mode: models.RelationshipUpdateModeRemove}})
			return err
		},
		"remove sub groups": func(ctx context.Context, ids []int) error {
			return db.Group().RemoveSubGroups(ctx, groupIDs[groupIdxWithChild], ids)
		},
		"gallery images": func(ctx context.Context, ids []int) error {
			return db.Gallery().RemoveImages(ctx, galleryIDs[galleryIdxWithImage], ids...)
		},
	}

	for name, remove := range removes {
		for _, ids := range [][]int{nil, {}} {
			var err, after error
			var tagsBefore, tagsAfter []int
			runRollbackTxn(t, func(ctx context.Context) error {
				var getErr error
				tagsBefore, getErr = db.Scene().GetTagIDs(ctx, sceneIDs[sceneIdxWithTwoTags])
				if getErr != nil {
					return getErr
				}
				err = remove(ctx, ids)
				// the transaction can still be used
				tagsAfter, after = db.Scene().GetTagIDs(ctx, sceneIDs[sceneIdxWithTwoTags])
				return nil
			})
			assert.NoError(t, err, "%s, %#v", name, ids)
			assert.NoError(t, after, "%s, %#v: transaction afterwards", name, ids)
			assert.Equal(t, tagsBefore, tagsAfter, "%s, %#v", name, ids)
		}
	}
}
