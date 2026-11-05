//go:build integration

package database_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestPerformerStudioStatistics(t *testing.T) {
	for _, statistic := range []string{"o_scene", "o_audio", "o_image", "o_gallery", "play_count", "scenes_duration", "scenes_size", "audios_duration", "audios_size"} {
		runWithRollbackTxn(t, statistic, func(t *testing.T, ctx context.Context) {
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
			sequence := 0
			add := func(person, studioID, value int) {
				sequence++
				ids := models.NewRelatedIDs([]int{person})
				dates := make([]time.Time, value)
				for i := range dates {
					dates[i] = time.Unix(int64(sequence*1000+i), 0).UTC()
				}
				switch statistic {
				case "o_image":
					im := models.NewImage()
					im.StudioID, im.PerformerIDs, im.OCounter = &studioID, ids, value
					require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &im}))
				case "o_gallery":
					g := models.NewGallery()
					g.StudioID, g.PerformerIDs, g.OCounter = &studioID, ids, value
					require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &g}))
				case "o_audio", "audios_duration", "audios_size":
					a := models.NewAudio()
					a.StudioID, a.PerformerIDs = &studioID, ids
					var fileIDs []models.FileID
					if statistic != "o_audio" {
						f := &models.AudioFile{
							BaseFile: &models.BaseFile{Basename: fmt.Sprintf("%s-%d", t.Name(), sequence), ParentFolderID: folderIDs[folderIdxWithSceneFiles], Size: int64(value)},
							Duration: float64(value),
						}
						require.NoError(t, db.File().Create(ctx, f))
						fileIDs = []models.FileID{f.ID}
					}
					require.NoError(t, db.Audio().Create(ctx, &a, fileIDs))
					if statistic == "o_audio" {
						_, err := db.Audio().AddO(ctx, a.ID, dates)
						require.NoError(t, err)
					}
				default:
					s := models.NewScene()
					s.StudioID, s.PerformerIDs = &studioID, ids
					var fileIDs []models.FileID
					if statistic == "scenes_duration" || statistic == "scenes_size" {
						f := &models.VideoFile{
							BaseFile: &models.BaseFile{Basename: fmt.Sprintf("%s-%d", t.Name(), sequence), ParentFolderID: folderIDs[folderIdxWithSceneFiles], Size: int64(value)},
							Duration: float64(value),
						}
						require.NoError(t, db.File().Create(ctx, f))
						fileIDs = []models.FileID{f.ID}
					}
					require.NoError(t, db.Scene().Create(ctx, &s, fileIDs))
					switch statistic {
					case "o_scene":
						_, err := db.Scene().AddO(ctx, s.ID, dates)
						require.NoError(t, err)
					case "play_count":
						_, err := db.Scene().AddViews(ctx, s.ID, dates)
						require.NoError(t, err)
					}
				}
			}
			add(people[0], studio, 1)
			add(people[0], child, 2)
			add(people[0], other, 100)
			add(people[1], studio, 2)
			add(people[2], other, 3)
			filter := &models.PerformerFilterType{
				Name:        &models.StringCriterionInput{Value: t.Name(), Modifier: models.CriterionModifierIncludes},
				StudioScope: &models.StudioScopeInput{ID: strconv.Itoa(studio)},
			}
			sort := statistic
			if statistic[:2] == "o_" {
				sort = "o_counter"
			}
			find := &models.FindFilterType{Sort: &sort}
			queryIDs := func() []int {
				ret, _, err := db.Performer().Query(ctx, filter, find)
				require.NoError(t, err)
				ids := make([]int, len(ret))
				for i, p := range ret {
					ids[i] = p.ID
				}
				return ids
			}
			assert.Equal(t, []int{people[2], people[0], people[1]}, queryIDs(), "sort excludes other studios")
			one := 1
			find.PerPage = &one
			assert.Equal(t, []int{people[2]}, queryIDs(), "scope applies before pagination")
			find.PerPage = nil
			if sort == "o_counter" || sort == "play_count" {
				setCount := func(f *models.PerformerFilterType, count *models.IntCriterionInput) {
					if sort == "o_counter" {
						f.OCounter = count
					} else {
						f.PlayCount = count
					}
				}
				setCount(filter, &models.IntCriterionInput{Value: 1, Modifier: models.CriterionModifierEquals})
				assert.Equal(t, []int{people[0]}, queryIDs())
				count, err := db.Performer().QueryCount(ctx, filter, nil)
				require.NoError(t, err)
				assert.Equal(t, 1, count)
				setCount(filter, &models.IntCriterionInput{Value: 0, Modifier: models.CriterionModifierEquals})
				assert.Equal(t, []int{people[2]}, queryIDs(), "no activity in this studio is zero")
				setCount(filter, nil)
				filter.And = &models.PerformerFilterType{}
				setCount(filter.And, &models.IntCriterionInput{Value: 1, Modifier: models.CriterionModifierEquals})
				assert.Equal(t, []int{people[0]}, queryIDs(), "compound filters inherit studio scope")
				filter.And = nil
			}
			if sort == "o_counter" {
				count, err := db.Performer().OCount(ctx, people[0], filter.StudioScope)
				require.NoError(t, err)
				assert.Equal(t, 1, count, "card and filter use the same scope")
				count, err = db.Performer().OCount(models.WithUserID(ctx, 0), people[0], filter.StudioScope)
				require.NoError(t, err)
				assert.Zero(t, count, "another user's activity is excluded")
			}
			allChildren := -1
			filter.StudioScope.Depth = &allChildren
			assert.Equal(t, []int{people[2], people[1], people[0]}, queryIDs(), "child studio activity follows depth")
			if sort == "o_counter" {
				count, err := db.Performer().OCount(ctx, people[0], filter.StudioScope)
				require.NoError(t, err)
				assert.Equal(t, 3, count)
			}
			filter.StudioScope = nil
			assert.Equal(t, []int{people[1], people[2], people[0]}, queryIDs(), "ordinary lists retain global totals")
		})
	}
}
