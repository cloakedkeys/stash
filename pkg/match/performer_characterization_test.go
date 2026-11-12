package match

import (
	"context"
	"slices"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
)

func TestPathToPerformers_Characterization(t *testing.T) {
	performers := []*models.Performer{
		{ID: 1, Name: "Jane"},
		{ID: 2, Name: "Jane Doe"},
		{ID: 3, Name: "伏字"},
		{ID: 4, Name: "Q"},
	}

	for _, tc := range []struct {
		path string
		want []int
	}{
		{"/media/Jane Doe.mp4", []int{1, 2}},
		{"/media/Jane.Doe.mp4", []int{1, 2}},
		{"/media/Jane-Doe.mp4", []int{1, 2}},
		{"/media/Jane_Doe.mp4", []int{1, 2}},
		{"/media/jAnE_dOe.mp4", []int{1, 2}},
		{"/media/Janet.mp4", nil},
		{"/media/JaneDoe.mp4", []int{2}},
		{"/media/伏字.mp4", []int{3}},
		{"/media/伏字Jane.mp4", nil},
		// Owner decision for TAG-1: skip one-letter names in both directions.
		{"/media/Q.mp4", nil},
	} {
		t.Run(tc.path, func(t *testing.T) {
			db := mocks.NewDatabase()
			primePerformerMock(db.Performer, performers)
			// Unlike the SQL prefilter, this mock supplies every candidate.
			// The preloaded case exercises the real in-memory prefix lookup.
			cache := &Cache{}
			if err := cache.PreloadPerformers(context.Background(), db.Performer); err != nil {
				t.Fatal(err)
			}
			got, err := PathToPerformers(context.Background(), tc.path, db.Performer, cache, false)
			if err != nil {
				t.Fatal(err)
			}
			if ids := perfIDs(got); !slices.Equal(ids, tc.want) {
				t.Errorf("got %v, want %v", ids, tc.want)
			}
		})
	}
}
