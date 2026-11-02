//go:build integration

package database_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Runs against SQLite normally and PostgreSQL when PGSQL_TEST is set.
func TestPerformerStudioScope(t *testing.T) {
	for _, media := range []string{"scene", "audio", "image", "gallery"} {
		runWithRollbackTxn(t, media, func(t *testing.T, ctx context.Context) {
			createStudio := func(name string, parent *int) int {
				s := models.NewStudio()
				s.Name, s.ParentID = t.Name()+name, parent
				require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: &s}))
				return s.ID
			}
			studio := createStudio(" studio", nil)
			child := createStudio(" child", &studio)
			other := createStudio(" other", nil)
			var people []int
			for _, name := range []string{" A", " B", " C"} {
				p := models.NewPerformer()
				p.Name = t.Name() + name
				require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
				people = append(people, p.ID)
			}
			add := func(person, studioID int, count int) {
				for i := 0; i < count; i++ {
					ids := models.NewRelatedIDs([]int{person})
					switch media {
					case "scene":
						s := models.NewScene()
						s.StudioID, s.PerformerIDs = &studioID, ids
						require.NoError(t, db.Scene().Create(ctx, &s, nil))
					case "audio":
						a := models.NewAudio()
						a.StudioID, a.PerformerIDs = &studioID, ids
						require.NoError(t, db.Audio().Create(ctx, &a, nil))
					case "image":
						im := models.NewImage()
						im.StudioID, im.PerformerIDs = &studioID, ids
						require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &im}))
					case "gallery":
						g := models.NewGallery()
						g.StudioID, g.PerformerIDs = &studioID, ids
						require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &g}))
					}
				}
			}
			add(people[0], studio, 1)
			add(people[0], child, 2)
			add(people[0], other, 3)
			add(people[1], studio, 2)
			add(people[2], other, 3)
			filter := &models.PerformerFilterType{
				Name:        &models.StringCriterionInput{Value: t.Name(), Modifier: models.CriterionModifierIncludes},
				StudioScope: &models.StudioScopeInput{ID: strconv.Itoa(studio)},
			}
			setCount := func(f *models.PerformerFilterType, c *models.IntCriterionInput) {
				switch media {
				case "scene":
					f.SceneCount = c
				case "audio":
					f.AudioCount = c
				case "image":
					f.ImageCount = c
				case "gallery":
					f.GalleryCount = c
				}
			}
			queryIDs := func(find *models.FindFilterType) []int {
				ret, _, err := db.Performer().Query(ctx, filter, find)
				require.NoError(t, err)
				ids := make([]int, len(ret))
				for i, p := range ret {
					ids[i] = p.ID
				}
				return ids
			}
			setCount(filter, &models.IntCriterionInput{Value: 1, Modifier: models.CriterionModifierEquals})
			assert.Equal(t, []int{people[0]}, queryIDs(nil), "other studios must not contribute to count filters")
			count, err := db.Performer().QueryCount(ctx, filter, nil)
			require.NoError(t, err)
			assert.Equal(t, 1, count)
			setCount(filter, &models.IntCriterionInput{Value: 0, Modifier: models.CriterionModifierEquals})
			assert.Equal(t, []int{people[2]}, queryIDs(nil), "zero in this studio is different from zero globally")
			setCount(filter, nil)
			sub := &models.PerformerFilterType{}
			setCount(sub, &models.IntCriterionInput{Value: 1, Modifier: models.CriterionModifierEquals})
			filter.And = sub
			assert.Equal(t, []int{people[0]}, queryIDs(nil), "compound filters inherit the studio scope")
			filter.And = nil
			sort := map[string]string{"scene": "scenes_count", "audio": "audios_count", "image": "images_count", "gallery": "galleries_count"}[media]
			find := &models.FindFilterType{Sort: &sort}
			assert.Equal(t, []int{people[2], people[0], people[1]}, queryIDs(find))
			one := 1
			find.PerPage = &one
			assert.Equal(t, []int{people[2]}, queryIDs(find), "scope must apply before pagination")
			find.PerPage = nil
			allChildren := -1
			filter.StudioScope.Depth = &allChildren
			assert.Equal(t, []int{people[2], people[1], people[0]}, queryIDs(find))
			filter.StudioScope = nil
			assert.Equal(t, []int{people[1], people[2], people[0]}, queryIDs(find), "ordinary performer lists keep global counts")
			otherStudio := &models.HierarchicalMultiCriterionInput{
				Value: []string{strconv.Itoa(other)}, Modifier: models.CriterionModifierIncludes,
			}
			switch media {
			case "scene":
				filter.ScenesFilter = &models.SceneFilterType{Studios: otherStudio}
			case "audio":
				filter.AudiosFilter = &models.AudioFilterType{Studios: otherStudio}
			case "image":
				filter.ImagesFilter = &models.ImageFilterType{Studios: otherStudio}
			case "gallery":
				filter.GalleriesFilter = &models.GalleryFilterType{Studios: otherStudio}
			}
			assert.Equal(t, []int{people[0], people[2]}, queryIDs(nil))
			filter.StudioScope = &models.StudioScopeInput{ID: strconv.Itoa(studio)}
			assert.Empty(t, queryIDs(nil), "related-media filters cannot match another studio's media")
		})
	}
}
