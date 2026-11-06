//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2225: the performer Groups filter ignored "includes all", so it returned
// every performer. Like the other "includes all" filters it must return only
// the performers in every chosen group, where a sub-group counts as its
// parent when sub-groups are included.
func TestPerformerGroupsIncludesAll(t *testing.T) {
	type data struct {
		group     map[string]int
		performer map[string]int
	}
	setup := func(t *testing.T, ctx context.Context) data {
		d := data{group: map[string]int{}, performer: map[string]int{}}
		for _, n := range []string{"G1", "G2", "G2 sub", "G3"} {
			g := models.Group{Name: "includes all group " + n}
			require.NoError(t, db.Group().Create(ctx, &g))
			d.group[n] = g.ID
		}
		_, _, err := db.ExecSQL(ctx, fmt.Sprintf("INSERT INTO groups_relations (containing_id, sub_id, order_index) VALUES (%d, %d, 0)", d.group["G2"], d.group["G2 sub"]), nil)
		require.NoError(t, err)

		// each performer's scenes, one group per scene
		scenes := map[string][]string{
			"in G1 and G2":     {"G1", "G2"},
			"in G1 and G2 sub": {"G1", "G2 sub"},
			"in G1 only":       {"G1"},
			"in G2 only":       {"G2"},
			"in G3":            {"G3"},
		}
		for n, groups := range scenes {
			p := models.Performer{Name: "includes all performer " + n}
			require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
			d.performer[n] = p.ID
			for _, g := range groups {
				s := models.Scene{
					Title:        "includes all scene " + n + " " + g,
					Groups:       models.NewRelatedGroups([]models.GroupsScenes{{GroupID: d.group[g]}}),
					PerformerIDs: models.NewRelatedIDs([]int{p.ID}),
				}
				require.NoError(t, db.Scene().Create(ctx, &s, nil))
			}
		}
		return d
	}

	names := func(d data, performers []*models.Performer) []string {
		ret := []string{}
		for _, p := range performers {
			found := false
			for n, id := range d.performer {
				if p.ID == id {
					ret = append(ret, n)
					found = true
				}
			}
			if !found {
				ret = append(ret, "other performer "+strconv.Itoa(p.ID))
			}
		}
		sort.Strings(ret)
		return ret
	}

	none, one, all := 0, 1, -1
	tests := []struct {
		name     string
		modifier models.CriterionModifier
		groups   []string
		depth    *int
		want     []string
	}{
		{"includes all, no sub-groups", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, &none, []string{"in G1 and G2"}},
		{"includes all, no depth given", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, nil, []string{"in G1 and G2"}},
		{"includes all, all sub-groups", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, &all, []string{"in G1 and G2", "in G1 and G2 sub"}},
		{"includes all, one level of sub-groups", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, &one, []string{"in G1 and G2", "in G1 and G2 sub"}},
		{"includes all, one group", models.CriterionModifierIncludesAll, []string{"G2"}, &all, []string{"in G1 and G2", "in G1 and G2 sub", "in G2 only"}},
		// unchanged
		{"includes, all sub-groups", models.CriterionModifierIncludes, []string{"G1", "G2"}, &all, []string{"in G1 and G2", "in G1 and G2 sub", "in G1 only", "in G2 only"}},
		{"includes, no sub-groups", models.CriterionModifierIncludes, []string{"G2"}, &none, []string{"in G1 and G2", "in G2 only"}},
		{"includes, two groups, no sub-groups", models.CriterionModifierIncludes, []string{"G1", "G2"}, &none, []string{"in G1 and G2", "in G1 and G2 sub", "in G1 only", "in G2 only"}},
	}

	for _, tt := range tests {
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			d := setup(t, ctx)
			var values []string
			for _, g := range tt.groups {
				values = append(values, strconv.Itoa(d.group[g]))
			}
			f := &models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{Value: values, Modifier: tt.modifier, Depth: tt.depth}}
			performers, _, err := db.Performer().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			assert.Equal(t, tt.want, names(d, performers))
		})
	}
}
