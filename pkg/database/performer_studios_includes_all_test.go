//go:build integration
// +build integration

package database_test

import (
	"context"
	"sort"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2234: the performer Studios filter ignored "includes all", so it returned
// every performer. It must return only the performers with scenes from every
// chosen studio, where a sub-studio counts as its parent when sub-studios are
// included.
func TestPerformerStudiosIncludesAll(t *testing.T) {
	type data struct {
		studio    map[string]int
		performer map[string]int
	}
	setup := func(t *testing.T, ctx context.Context) data {
		d := data{studio: map[string]int{}, performer: map[string]int{}}
		for _, n := range []string{"A", "B"} {
			s := models.Studio{Name: "includes all studio " + n}
			require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: &s}))
			d.studio[n] = s.ID
		}
		sub := models.Studio{Name: "includes all studio A sub", ParentID: &[]int{d.studio["A"]}[0]}
		require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: &sub}))
		d.studio["A sub"] = sub.ID

		// each performer's scenes, one studio per scene
		scenes := map[string][]string{
			"A and B":     {"A", "B"},
			"A sub and B": {"A sub", "B"},
			"A only":      {"A"},
			"B only":      {"B"},
		}
		for n, studios := range scenes {
			p := models.Performer{Name: "includes all studio performer " + n}
			require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
			d.performer[n] = p.ID
			for _, st := range studios {
				s := models.Scene{
					Title:        "includes all studio scene " + n + " " + st,
					StudioID:     &[]int{d.studio[st]}[0],
					PerformerIDs: models.NewRelatedIDs([]int{p.ID}),
				}
				require.NoError(t, db.Scene().Create(ctx, &s, nil))
			}
		}
		return d
	}

	// the names of this test's performers in the result; with all set, any
	// other performer in the result is listed too
	names := func(d data, performers []*models.Performer, all bool) []string {
		ret := []string{}
		for _, p := range performers {
			found := false
			for n, id := range d.performer {
				if p.ID == id {
					ret = append(ret, n)
					found = true
				}
			}
			if !found && all {
				ret = append(ret, "other performer "+strconv.Itoa(p.ID))
			}
		}
		sort.Strings(ret)
		return ret
	}

	none, all := 0, -1
	tests := []struct {
		name     string
		modifier models.CriterionModifier
		studios  []string
		depth    *int
		// excludes returns other performers too, so only this test's are compared
		onlyOurs bool
		want     []string
	}{
		{"includes all, no sub-studios", models.CriterionModifierIncludesAll, []string{"A", "B"}, &none, false, []string{"A and B"}},
		{"includes all, no depth given", models.CriterionModifierIncludesAll, []string{"A", "B"}, nil, false, []string{"A and B"}},
		{"includes all, all sub-studios", models.CriterionModifierIncludesAll, []string{"A", "B"}, &all, false, []string{"A and B", "A sub and B"}},
		// unchanged
		{"includes, all sub-studios", models.CriterionModifierIncludes, []string{"A", "B"}, &all, false, []string{"A and B", "A only", "A sub and B", "B only"}},
		{"includes, no sub-studios", models.CriterionModifierIncludes, []string{"A"}, &none, false, []string{"A and B", "A only"}},
		{"excludes, all sub-studios", models.CriterionModifierExcludes, []string{"A"}, &all, true, []string{"B only"}},
		{"excludes, no sub-studios", models.CriterionModifierExcludes, []string{"A"}, &none, true, []string{"A sub and B", "B only"}},
	}

	for _, tt := range tests {
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			d := setup(t, ctx)
			var values []string
			for _, s := range tt.studios {
				values = append(values, strconv.Itoa(d.studio[s]))
			}
			f := &models.PerformerFilterType{Studios: &models.HierarchicalMultiCriterionInput{Value: values, Modifier: tt.modifier, Depth: tt.depth}}
			performers, _, err := db.Performer().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			assert.Equal(t, tt.want, names(d, performers, !tt.onlyOurs))
		})
	}
}
