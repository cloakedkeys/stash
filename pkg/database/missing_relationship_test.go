//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Runs against SQLite normally and PostgreSQL when PGSQL_TEST is set.
func TestMissingRelationshipError(t *testing.T) {
	for _, entity := range []string{"scene", "audio", "image", "gallery", "performer", "studio", "group", "story"} {
		for _, relationship := range []string{"tag", "performer"} {
			if relationship == "performer" && entity != "scene" && entity != "audio" && entity != "image" && entity != "gallery" {
				continue
			}
			for _, mode := range []models.RelationshipUpdateMode{
				models.RelationshipUpdateModeSet,
				models.RelationshipUpdateModeAdd,
				models.RelationshipUpdateModeRemove,
			} {
				runWithRollbackTxn(t, entity+"/"+relationship+"/"+string(mode), func(t *testing.T, ctx context.Context) {
					var update func(*models.UpdateIDs) error
					var load func() ([]int, error)
					switch entity {
					case "scene":
						id := sceneIDs[0]
						update = func(ids *models.UpdateIDs) error {
							partial := models.ScenePartial{TagIDs: ids}
							if relationship == "performer" {
								partial.TagIDs, partial.PerformerIDs = nil, ids
							}
							_, err := db.Scene().UpdatePartial(ctx, id, partial)
							return err
						}
						load = func() ([]int, error) {
							if relationship == "performer" {
								return db.Scene().GetPerformerIDs(ctx, id)
							}
							return db.Scene().GetTagIDs(ctx, id)
						}
					case "audio":
						id := audioIDs[0]
						update = func(ids *models.UpdateIDs) error {
							partial := models.AudioPartial{TagIDs: ids}
							if relationship == "performer" {
								partial.TagIDs, partial.PerformerIDs = nil, ids
							}
							_, err := db.Audio().UpdatePartial(ctx, id, partial)
							return err
						}
						load = func() ([]int, error) {
							if relationship == "performer" {
								return db.Audio().GetPerformerIDs(ctx, id)
							}
							return db.Audio().GetTagIDs(ctx, id)
						}
					case "image":
						id := imageIDs[0]
						update = func(ids *models.UpdateIDs) error {
							partial := models.ImagePartial{TagIDs: ids}
							if relationship == "performer" {
								partial.TagIDs, partial.PerformerIDs = nil, ids
							}
							_, err := db.Image().UpdatePartial(ctx, id, partial)
							return err
						}
						load = func() ([]int, error) {
							if relationship == "performer" {
								return db.Image().GetPerformerIDs(ctx, id)
							}
							return db.Image().GetTagIDs(ctx, id)
						}
					case "gallery":
						id := galleryIDs[0]
						update = func(ids *models.UpdateIDs) error {
							partial := models.GalleryPartial{TagIDs: ids}
							if relationship == "performer" {
								partial.TagIDs, partial.PerformerIDs = nil, ids
							}
							_, err := db.Gallery().UpdatePartial(ctx, id, partial)
							return err
						}
						load = func() ([]int, error) {
							if relationship == "performer" {
								return db.Gallery().GetPerformerIDs(ctx, id)
							}
							return db.Gallery().GetTagIDs(ctx, id)
						}
					case "performer":
						id := performerIDs[0]
						update = func(ids *models.UpdateIDs) error {
							partial := models.PerformerPartial{TagIDs: ids}
							_, err := db.Performer().UpdatePartial(ctx, id, partial)
							return err
						}
						load = func() ([]int, error) {
							return db.Performer().GetTagIDs(ctx, id)
						}
					case "studio":
						id := studioIDs[0]
						update = func(ids *models.UpdateIDs) error {
							partial := models.StudioPartial{ID: id, TagIDs: ids}
							_, err := db.Studio().UpdatePartial(ctx, partial)
							return err
						}
						load = func() ([]int, error) {
							return db.Studio().GetTagIDs(ctx, id)
						}
					case "group":
						id := groupIDs[0]
						update = func(ids *models.UpdateIDs) error {
							partial := models.GroupPartial{TagIDs: ids}
							_, err := db.Group().UpdatePartial(ctx, id, partial)
							return err
						}
						load = func() ([]int, error) {
							return db.Group().GetTagIDs(ctx, id)
						}
					case "story":
						s := models.NewStory()
						s.Title = t.Name()
						require.NoError(t, db.Story().Create(ctx, &s, nil))
						id := s.ID
						update = func(ids *models.UpdateIDs) error {
							partial := models.StoryPartial{TagIDs: ids}
							_, err := db.Story().UpdatePartial(ctx, id, partial)
							return err
						}
						load = func() ([]int, error) {
							return db.Story().GetTagIDs(ctx, id)
						}
					}

					valid := tagIDs[0]
					if relationship == "performer" {
						valid = performerIDs[0]
					}
					const missing = -1
					if relationship == "tag" {
						found, err := db.Tag().Find(ctx, missing)
						require.NoError(t, err)
						require.Nil(t, found)
					} else {
						found, err := db.Performer().Find(ctx, missing)
						require.NoError(t, err)
						require.Nil(t, found)
					}

					// Valid and duplicate IDs must still work in all three modes.
					require.NoError(t, update(&models.UpdateIDs{Mode: models.RelationshipUpdateModeSet, IDs: []int{valid}}))
					require.NoError(t, update(&models.UpdateIDs{Mode: mode, IDs: []int{valid, valid}}))
					got, err := load()
					require.NoError(t, err)
					if mode == models.RelationshipUpdateModeRemove {
						assert.Empty(t, got)
					} else {
						assert.Equal(t, []int{valid}, got)
					}

					if mode == models.RelationshipUpdateModeRemove {
						// Removing a stale reference remains a harmless no-op.
						require.NoError(t, update(&models.UpdateIDs{Mode: mode, IDs: []int{missing}}))
					} else {
						err := update(&models.UpdateIDs{Mode: mode, IDs: []int{valid, missing}})
						require.EqualError(t, err, relationship+" with id -1 not found")
					}
				})
			}
		}
	}
}
