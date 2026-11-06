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
		for _, n := range []string{"G1", "G2", "G2 sub", "G2 sub sub", "G3"} {
			g := models.Group{Name: "includes all group " + n}
			require.NoError(t, db.Group().Create(ctx, &g))
			d.group[n] = g.ID
		}
		// G2 -> G2 sub -> G2 sub sub
		for i, r := range [][2]string{{"G2", "G2 sub"}, {"G2 sub", "G2 sub sub"}} {
			_, _, err := db.ExecSQL(ctx, fmt.Sprintf("INSERT INTO groups_relations (containing_id, sub_id, order_index) VALUES (%d, %d, %d)", d.group[r[0]], d.group[r[1]], i), nil)
			require.NoError(t, err)
		}

		// each performer's scenes, one group per scene
		scenes := map[string][]string{
			"in G1 and G2":         {"G1", "G2"},
			"in G1 and G2 sub":     {"G1", "G2 sub"},
			"in G1 and G2 sub sub": {"G1", "G2 sub sub"},
			"in G1 only":           {"G1"},
			"in G2 only":           {"G2"},
			"in G3":                {"G3"},
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

	none, one, all := 0, 1, -1
	tests := []struct {
		name     string
		modifier models.CriterionModifier
		groups   []string
		depth    *int
		// excludes returns other performers too, so only this test's are compared
		onlyOurs bool
		want     []string
	}{
		{"includes all, no sub-groups", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, &none, false, []string{"in G1 and G2"}},
		{"includes all, no depth given", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, nil, false, []string{"in G1 and G2"}},
		{"includes all, all sub-groups", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, &all, false, []string{"in G1 and G2", "in G1 and G2 sub", "in G1 and G2 sub sub"}},
		// G2 sub sub is two levels down, so one level leaves it out
		{"includes all, one level of sub-groups", models.CriterionModifierIncludesAll, []string{"G1", "G2"}, &one, false, []string{"in G1 and G2", "in G1 and G2 sub"}},
		{"includes all, one group", models.CriterionModifierIncludesAll, []string{"G2"}, &all, false, []string{"in G1 and G2", "in G1 and G2 sub", "in G1 and G2 sub sub", "in G2 only"}},
		// unchanged on SQLite; on PostgreSQL only the first of several groups
		// was used when sub-groups were off
		{"includes, all sub-groups", models.CriterionModifierIncludes, []string{"G1", "G2"}, &all, false, []string{"in G1 and G2", "in G1 and G2 sub", "in G1 and G2 sub sub", "in G1 only", "in G2 only"}},
		{"includes, no sub-groups", models.CriterionModifierIncludes, []string{"G2"}, &none, false, []string{"in G1 and G2", "in G2 only"}},
		{"includes, two groups, no sub-groups", models.CriterionModifierIncludes, []string{"G1", "G2"}, &none, false, []string{"in G1 and G2", "in G1 and G2 sub", "in G1 and G2 sub sub", "in G1 only", "in G2 only"}},
		{"excludes, two groups, no sub-groups", models.CriterionModifierExcludes, []string{"G1", "G2"}, &none, true, []string{"in G3"}},
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
			assert.Equal(t, tt.want, names(d, performers, !tt.onlyOurs))
		})
	}
}
