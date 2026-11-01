//go:build integration
// +build integration

package database_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/match"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrapedRelationshipWhitespace(t *testing.T) {
	for _, alias := range []bool{false, true} {
		name := "name"
		if alias {
			name = "alias"
		}
		runWithRollbackTxn(t, name, func(t *testing.T, ctx context.Context) {
			performer := models.NewPerformer()
			performer.Name = "Identify Performer"
			performer.Aliases = models.NewRelatedStrings([]string{"Identify Alias"})
			require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &performer}))

			studio := models.NewCreateStudioInput()
			studio.Name = "Identify Studio"
			studio.Aliases = models.NewRelatedStrings([]string{"Identify Studio Alias"})
			require.NoError(t, db.Studio().Create(ctx, &studio))

			performerName := performer.Name
			studioName := studio.Name
			if alias {
				performerName = "Identify Alias"
				studioName = "Identify Studio Alias"
			}
			// Use the same whitespace normalization as ToPerformer and ToStudio.
			performerName = " \t" + performerName + "\r\n\u00a0"
			scene := &models.ScrapedScene{
				Performers: []*models.ScrapedPerformer{{Name: &performerName}},
				Studio:     &models.ScrapedStudio{Name: "\u00a0" + studioName + " \n"},
			}
			matcher := match.SceneRelationships{PerformerFinder: db.Performer(), StudioFinder: db.Studio()}
			require.NoError(t, matcher.MatchRelationships(ctx, scene, ""))
			wantPerformerID := strconv.Itoa(performer.ID)
			wantStudioID := strconv.Itoa(studio.ID)
			assert.Equal(t, &wantPerformerID, scene.Performers[0].StoredID)
			assert.Equal(t, &wantStudioID, scene.Studio.StoredID)
		})
	}
}

func TestScrapedPerformerSameName(t *testing.T) {
	first := "first"
	unknown := "unknown"
	tests := []struct {
		name           string
		disambiguation *string
		wantMatch      bool
	}{
		{name: "no disambiguation", wantMatch: true},
		{name: "matching disambiguation", disambiguation: &first, wantMatch: true},
		{name: "unknown disambiguation", disambiguation: &unknown},
	}
	for _, tt := range tests {
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			name := "Identify Same Name"
			var wantID int
			wantDisambiguation := ""
			if tt.disambiguation != nil {
				wantDisambiguation = *tt.disambiguation
			}
			for _, disambiguation := range []string{"", "first", "second"} {
				p := models.NewPerformer()
				p.Name = name
				p.Disambiguation = disambiguation
				require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
				if disambiguation == wantDisambiguation {
					wantID = p.ID
				}
			}
			// A case-insensitive lookup can also return a distinct, legal name.
			other := models.NewPerformer()
			other.Name = "IDENTIFY SAME NAME"
			require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &other}))

			scraped := &models.ScrapedPerformer{Name: &name, Disambiguation: tt.disambiguation}
			require.NoError(t, match.ScrapedPerformer(ctx, db.Performer(), scraped, ""))
			if tt.wantMatch {
				require.NotNil(t, scraped.StoredID)
				require.Equal(t, strconv.Itoa(wantID), *scraped.StoredID)
			} else {
				require.Nil(t, scraped.StoredID)
			}
		})
	}
}

func TestScrapedStudioExistingParent(t *testing.T) {
	for _, matchedChild := range []bool{false, true} {
		runWithRollbackTxn(t, strconv.FormatBool(matchedChild), func(t *testing.T, ctx context.Context) {
			parent := models.NewCreateStudioInput()
			parent.Name = "Identify Existing Parent"
			require.NoError(t, db.Studio().Create(ctx, &parent))

			child := &models.ScrapedStudio{
				Name:   "Identify New Child",
				Parent: &models.ScrapedStudio{Name: parent.Name},
			}
			if matchedChild {
				id := "123"
				child.StoredID = &id
			}
			// Scraper postprocessing calls ScrapedStudio directly; the tagger
			// also needs the parent matched when the child already has an ID.
			require.NoError(t, match.ScrapedStudio(ctx, db.Studio(), child, ""))
			require.NotNil(t, child.Parent.StoredID)
			assert.Equal(t, strconv.Itoa(parent.ID), *child.Parent.StoredID)
		})
	}
}
