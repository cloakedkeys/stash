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
	sceneID := sceneIDs[sceneIdxWithTwoTags]
	performerSceneID := sceneIDs[sceneIdxWithTwoPerformers]
	containingID := groupIDs[groupIdxWithChild]
	subID := groupIDs[groupIdxWithParent]
	galleryID := galleryIDs[galleryIdxWithImage]

	groupList := func(ids []int) []models.GroupIDDescription {
		groups := make([]models.GroupIDDescription, len(ids))
		for i, id := range ids {
			groups[i] = models.GroupIDDescription{GroupID: id}
		}
		return groups
	}

	type removeCase struct {
		// remove removes ids
		remove func(ctx context.Context, ids []int) error
		// links reads the links the remove works on
		links func(ctx context.Context) (interface{}, error)
	}

	sceneTags := func(ctx context.Context) (interface{}, error) { return db.Scene().GetTagIDs(ctx, sceneID) }
	subGroups := func(ctx context.Context) (interface{}, error) {
		return db.Group().GetSubGroupDescriptions(ctx, containingID)
	}

	cases := map[string]removeCase{
		"scene tags": {
			remove: func(ctx context.Context, ids []int) error {
				_, err := db.Scene().UpdatePartial(ctx, sceneID, models.ScenePartial{TagIDs: &models.UpdateIDs{IDs: ids, Mode: models.RelationshipUpdateModeRemove}})
				return err
			},
			links: sceneTags,
		},
		"scene performers": {
			remove: func(ctx context.Context, ids []int) error {
				_, err := db.Scene().UpdatePartial(ctx, performerSceneID, models.ScenePartial{PerformerIDs: &models.UpdateIDs{IDs: ids, Mode: models.RelationshipUpdateModeRemove}})
				return err
			},
			links: func(ctx context.Context) (interface{}, error) {
				return db.Scene().GetPerformerIDs(ctx, performerSceneID)
			},
		},
		"group sub groups": {
			remove: func(ctx context.Context, ids []int) error {
				_, err := db.Group().UpdatePartial(ctx, containingID, models.GroupPartial{SubGroups: &models.UpdateGroupDescriptions{Groups: groupList(ids), Mode: models.RelationshipUpdateModeRemove}})
				return err
			},
			links: subGroups,
		},
		"group containing groups": {
			remove: func(ctx context.Context, ids []int) error {
				_, err := db.Group().UpdatePartial(ctx, subID, models.GroupPartial{ContainingGroups: &models.UpdateGroupDescriptions{Groups: groupList(ids), Mode: models.RelationshipUpdateModeRemove}})
				return err
			},
			links: func(ctx context.Context) (interface{}, error) {
				return db.Group().GetContainingGroupDescriptions(ctx, subID)
			},
		},
		"remove sub groups": {
			remove: func(ctx context.Context, ids []int) error {
				return db.Group().RemoveSubGroups(ctx, containingID, ids)
			},
			links: subGroups,
		},
		"gallery images": {
			remove: func(ctx context.Context, ids []int) error {
				return db.Gallery().RemoveImages(ctx, galleryID, ids...)
			},
			links: func(ctx context.Context) (interface{}, error) { return db.Gallery().GetImageIDs(ctx, galleryID) },
		},
	}

	for name, c := range cases {
		for _, ids := range [][]int{nil, {}} {
			var err, readErr error
			var before, after interface{}
			runRollbackTxn(t, func(ctx context.Context) error {
				var getErr error
				before, getErr = c.links(ctx)
				if getErr != nil {
					return getErr
				}
				err = c.remove(ctx, ids)
				// the links are unchanged, and reading them shows the transaction still works
				after, readErr = c.links(ctx)
				return nil
			})
			assert.NotEmpty(t, before, "%s: the fixture should have links to keep", name)
			assert.NoError(t, err, "%s, %#v", name, ids)
			assert.NoError(t, readErr, "%s, %#v: transaction afterwards", name, ids)
			assert.Equal(t, before, after, "%s, %#v: links changed", name, ids)
		}
	}
}
