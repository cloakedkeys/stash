//go:build integration
// +build integration

package database_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// #2167: each level of an OR or NOT filter must select its items by itself.
// Conditions that join other tables were checked row by row in one joined
// query, so a NOT on them matched an item when any one of its joined rows
// failed the condition, and a condition that only filters through an inner
// join stopped filtering under OR. Every condition here is combined under
// AND, OR and NOT, alone and with grouped conditions, and each result is
// compared with the same set operation on the conditions' own results.
//
// #2172: two levels with the same kind of condition shared one join, so
// only one level's values were used, under AND as well.

type sceneCondition struct {
	name string
	set  func(f *models.SceneFilterType)
	// if set, the condition is only checked at the top level of a filter.
	// No condition needs this since sub-filter WITH clauses work (#2170).
	with bool
}

func (c sceneCondition) filter() *models.SceneFilterType {
	f := &models.SceneFilterType{}
	c.set(f)
	return f
}

// sameFields reports whether the conditions set a field in common, so they
// can't share one filter
func (c sceneCondition) sameFields(o sceneCondition) bool {
	a := reflect.ValueOf(c.filter()).Elem()
	b := reflect.ValueOf(o.filter()).Elem()
	for i := 0; i < a.NumField(); i++ {
		if !a.Field(i).IsZero() && !b.Field(i).IsZero() {
			return true
		}
	}
	return false
}

func sceneIDsSet(ctx context.Context, t *testing.T, f *models.SceneFilterType) (idSet, bool) {
	t.Helper()
	r, err := db.Scene().Query(ctx, models.SceneQueryOptions{
		QueryOptions: models.QueryOptions{FindFilter: allItems(), Count: true},
		SceneFilter:  f,
	})
	if err != nil {
		t.Errorf("query: %v", err)
		return nil, false
	}
	ret := idSet{}
	for _, id := range r.IDs {
		ret[id] = true
	}
	assert.Len(t, ret, len(r.IDs), "repeated ids")
	assert.Equal(t, len(r.IDs), r.Count, "count")
	return ret, true
}

func idStrings(ids ...int) []string {
	var ret []string
	for _, id := range ids {
		ret = append(ret, strconv.Itoa(id))
	}
	return ret
}

func TestJoinedConditionsUnderOperators(t *testing.T) {
	yes := true
	trueStr := "true"
	intCrit := func(v int) *models.IntCriterionInput {
		return &models.IntCriterionInput{Value: v, Modifier: models.CriterionModifierGreaterThan}
	}
	missing := func(v string) func(f *models.SceneFilterType) {
		return func(f *models.SceneFilterType) { f.IsMissing = &v }
	}

	base := sceneCondition{"title", func(f *models.SceneFilterType) { f.Title = regexCriterion(firstIndexesRegex) }, false}
	none := sceneCondition{"none", func(f *models.SceneFilterType) { f.Title = regexCriterion("^no such title$") }, false}

	conditions := []sceneCondition{
		{"performers", func(f *models.SceneFilterType) {
			f.Performers = &models.MultiCriterionInput{
				Value:    idStrings(performerIDs[performerIdx1WithScene], performerIDs[performerIdx2WithScene]),
				Modifier: models.CriterionModifierIncludes,
			}
		}, false},
		{"studios", func(f *models.SceneFilterType) {
			f.Studios = &models.HierarchicalMultiCriterionInput{
				Value:    idStrings(studioIDs[studioIdxWithScene]),
				Modifier: models.CriterionModifierIncludes,
			}
		}, false},
		{"tags", func(f *models.SceneFilterType) {
			f.Tags = &models.HierarchicalMultiCriterionInput{
				Value:    idStrings(tagIDs[tagIdx1WithScene], tagIDs[tagIdx2WithScene]),
				Modifier: models.CriterionModifierIncludes,
			}
		}, false},
		{"galleries", func(f *models.SceneFilterType) {
			f.Galleries = &models.MultiCriterionInput{
				Value:    idStrings(galleryIDs[galleryIdxWithScene]),
				Modifier: models.CriterionModifierIncludes,
			}
		}, false},
		{"groups", func(f *models.SceneFilterType) {
			f.Groups = &models.HierarchicalMultiCriterionInput{
				Value:    idStrings(groupIDs[groupIdxWithScene]),
				Modifier: models.CriterionModifierIncludes,
			}
		}, false},
		{"performer_count", func(f *models.SceneFilterType) { f.PerformerCount = intCrit(1) }, false},
		{"tag_count", func(f *models.SceneFilterType) { f.TagCount = intCrit(1) }, false},
		{"performer_tags", func(f *models.SceneFilterType) {
			f.PerformerTags = &models.HierarchicalMultiCriterionInput{
				Value:    idStrings(tagIDs[tagIdxWithPerformer]),
				Modifier: models.CriterionModifierIncludes,
			}
		}, false},
		{"performer_favorite", func(f *models.SceneFilterType) { f.PerformerFavorite = &yes }, false},
		{"performer_age", func(f *models.SceneFilterType) { f.PerformerAge = intCrit(0) }, false},
		{"duplicated phash", func(f *models.SceneFilterType) {
			f.Duplicated = &models.DuplicationCriterionInput{Phash: &yes}
		}, false},
		{"performers_filter", func(f *models.SceneFilterType) {
			f.PerformersFilter = &models.PerformerFilterType{FilterFavorites: &yes}
		}, false},
		{"tags_filter", func(f *models.SceneFilterType) {
			f.TagsFilter = &models.TagFilterType{Name: regexCriterion("^tag_[0-4]_")}
		}, false},
		{"markers_filter", func(f *models.SceneFilterType) {
			id := strconv.Itoa(tagIDs[tagIdxWithPrimaryMarkers])
			f.MarkersFilter = &models.SceneMarkerFilterType{TagID: &id}
		}, false},
		{"files_filter", func(f *models.SceneFilterType) {
			f.FilesFilter = &models.FileFilterType{Basename: regexCriterion(firstIndexesRegex)}
		}, false},
		{"path", func(f *models.SceneFilterType) { f.Path = regexCriterion("_00[01][0-4]_[A-Za-z]+$") }, false},
		{"url", func(f *models.SceneFilterType) { f.URL = regexCriterion(firstIndexesRegex) }, false},
		{"is_missing studio", missing("studio"), false},
		{"is_missing performers", missing("performers"), false},
		{"is_missing galleries", missing("galleries"), false},
		{"is_missing tags", missing("tags"), false},
		{"is_missing phash", missing("phash"), false},
		{"duration", func(f *models.SceneFilterType) { f.Duration = intCrit(1000) }, false},
		{"rating", func(f *models.SceneFilterType) { f.Rating100 = intCrit(0) }, false},
		{"stash_id_count", func(f *models.SceneFilterType) { f.StashIDCount = intCrit(0) }, false},
		{"resolution", func(f *models.SceneFilterType) {
			f.Resolution = &models.ResolutionCriterionInput{Value: models.ResolutionEnumLow, Modifier: models.CriterionModifierGreaterThan}
		}, false},
	}

	grouped := []sceneCondition{
		{"has_markers", func(f *models.SceneFilterType) { f.HasMarkers = &trueStr }, false},
		{"tags includes_all", func(f *models.SceneFilterType) {
			f.Tags = &models.HierarchicalMultiCriterionInput{
				Value:    idStrings(tagIDs[tagIdx1WithScene], tagIDs[tagIdx2WithScene]),
				Modifier: models.CriterionModifierIncludesAll,
			}
		}, false},
		{"performers includes_all", func(f *models.SceneFilterType) {
			f.Performers = &models.MultiCriterionInput{
				Value:    idStrings(performerIDs[performerIdx1WithScene], performerIDs[performerIdx2WithScene]),
				Modifier: models.CriterionModifierIncludesAll,
			}
		}, false},
	}

	withTxn(func(ctx context.Context) error {
		// the result of a condition on its own, at the top level
		sets := map[string]idSet{}
		all, _ := sceneIDsSet(ctx, t, &models.SceneFilterType{})
		for _, c := range append(append([]sceneCondition{base, none}, conditions...), grouped...) {
			s, _ := sceneIDsSet(ctx, t, c.filter())
			sets[c.name] = s
		}

		// the conditions must split the scenes for the checks to mean anything
		for _, c := range append(conditions, grouped...) {
			assert.NotEmpty(t, sets[c.name].sorted(), "%s finds nothing", c.name)
			assert.NotEqual(t, all.sorted(), sets[c.name].sorted(), "%s finds everything", c.name)
		}
		assert.Empty(t, sets[none.name].sorted())

		check := func(name string, want idSet, f *models.SceneFilterType) {
			t.Run(name, func(t *testing.T) {
				if got, ok := sceneIDsSet(ctx, t, f); ok {
					assert.Equal(t, want.sorted(), got.sorted())
				}
			})
		}

		// first OR / AND NOT second, each its own filter
		or := func(a, b sceneCondition) *models.SceneFilterType {
			f := a.filter()
			f.Or = b.filter()
			return f
		}
		not := func(a, b sceneCondition) *models.SceneFilterType {
			f := a.filter()
			f.Not = b.filter()
			return f
		}
		and := func(a, b sceneCondition) *models.SceneFilterType {
			f := a.filter()
			f.And = b.filter()
			return f
		}
		notOnly := func(a sceneCondition) *models.SceneFilterType {
			return &models.SceneFilterType{OperatorFilter: models.OperatorFilter[models.SceneFilterType]{Not: a.filter()}}
		}
		// both conditions in one filter
		both := func(a, b sceneCondition) sceneCondition {
			return sceneCondition{
				name: "{" + a.name + ", " + b.name + "}",
				set: func(f *models.SceneFilterType) {
					a.set(f)
					b.set(f)
				},
				with: a.with || b.with,
			}
		}

		for _, x := range conditions {
			X := sets[x.name]
			B := sets[base.name]
			check(x.name+" OR none", X, or(x, none))
			check(x.name+" AND title", setAnd(X, B), and(x, base))
			check(x.name+" OR title", setOr(X, B), or(x, base))
			check(x.name+" NOT title", setMinus(X, B), not(x, base))
			if !x.with {
				check("none OR "+x.name, X, or(none, x))
				check("NOT "+x.name, setMinus(all, X), notOnly(x))
				check("title OR "+x.name, setOr(B, X), or(base, x))
				check("title NOT "+x.name, setMinus(B, X), not(base, x))
			}

			for _, h := range grouped {
				H := sets[h.name]
				check(x.name+" OR "+h.name, setOr(X, H), or(x, h))
				check(x.name+" NOT "+h.name, setMinus(X, H), not(x, h))
				if !x.with {
					check(h.name+" OR "+x.name, setOr(H, X), or(h, x))
					check(h.name+" NOT "+x.name, setMinus(H, X), not(h, x))
				}

				if x.sameFields(h) {
					continue
				}
				xh := both(x, h)
				XH, _ := sceneIDsSet(ctx, t, xh.filter())
				check(xh.name+" OR none", XH, or(xh, none))
				if !x.with {
					check("none OR "+xh.name, XH, or(none, xh))
					check("NOT "+xh.name, setMinus(all, XH), notOnly(xh))
				}
			}
		}

		// the same kind of condition on both sides, with different values.
		// Both sides used to share one join alias, so only the first side's
		// values were used.
		tagsOf := func(idx int) sceneCondition {
			return sceneCondition{"tag " + strconv.Itoa(idx), func(f *models.SceneFilterType) {
				f.Tags = &models.HierarchicalMultiCriterionInput{Value: idStrings(tagIDs[idx]), Modifier: models.CriterionModifierIncludes}
			}, false}
		}
		performersOf := func(idx int) sceneCondition {
			return sceneCondition{"performer " + strconv.Itoa(idx), func(f *models.SceneFilterType) {
				f.Performers = &models.MultiCriterionInput{Value: idStrings(performerIDs[idx]), Modifier: models.CriterionModifierIncludes}
			}, false}
		}
		groupsOf := func(idx int) sceneCondition {
			return sceneCondition{"group " + strconv.Itoa(idx), func(f *models.SceneFilterType) {
				f.Groups = &models.HierarchicalMultiCriterionInput{Value: idStrings(groupIDs[idx]), Modifier: models.CriterionModifierIncludes}
			}, false}
		}
		sameKind := [][2]sceneCondition{
			{tagsOf(tagIdx1WithScene), tagsOf(tagIdx2WithScene)},
			{performersOf(performerIdxWithScene), performersOf(performerIdxWithTwoScenes)},
			{groupsOf(groupIdxWithScene), groupsOf(groupIdxWithParentAndScene)},
		}
		for _, p := range sameKind {
			a, b := p[0], p[1]
			A, _ := sceneIDsSet(ctx, t, a.filter())
			B, _ := sceneIDsSet(ctx, t, b.filter())
			assert.NotEmpty(t, A.sorted(), "%s finds nothing", a.name)
			assert.NotEmpty(t, B.sorted(), "%s finds nothing", b.name)
			assert.NotEqual(t, A.sorted(), B.sorted(), "%s and %s find the same", a.name, b.name)

			check(a.name+" AND "+b.name, setAnd(A, B), and(a, b))
			check(b.name+" AND "+a.name, setAnd(B, A), and(b, a))
			f3 := and(a, base)
			f3.And.And = b.filter()
			check(a.name+" AND (title AND "+b.name+")", setAnd(setAnd(A, sets[base.name]), B), f3)
			check(a.name+" OR "+b.name, setOr(A, B), or(a, b))
			check(a.name+" NOT "+b.name, setMinus(A, B), not(a, b))
			f := and(a, b)
			f.And.Or = none.filter()
			check(a.name+" AND ("+b.name+" OR none)", setAnd(A, B), f)
		}

		// nested filters
		cond := func(name string) sceneCondition {
			for _, c := range append(append([]sceneCondition{base}, conditions...), grouped...) {
				if c.name == name {
					return c
				}
			}
			t.Fatalf("no condition %s", name)
			return sceneCondition{}
		}
		withOp := func(f *models.SceneFilterType, set func(o *models.OperatorFilter[models.SceneFilterType])) *models.SceneFilterType {
			set(&f.OperatorFilter)
			return f
		}

		// title OR (performers AND NOT (has_markers OR studios))
		{
			inner := withOp(cond("performers").filter(), func(o *models.OperatorFilter[models.SceneFilterType]) {
				o.Not = or(cond("has_markers"), cond("studios"))
			})
			f := withOp(base.filter(), func(o *models.OperatorFilter[models.SceneFilterType]) { o.Or = inner })
			want := setOr(sets["title"], setMinus(sets["performers"], setOr(sets["has_markers"], sets["studios"])))
			check("title OR (performers AND NOT (has_markers OR studios))", want, f)
		}
		// NOT (has_markers OR (tags AND NOT performers includes_all))
		{
			inner := not(cond("tags"), cond("performers includes_all"))
			f := notOnly(cond("has_markers"))
			f.Not.Or = inner
			want := setMinus(all, setOr(sets["has_markers"], setMinus(sets["tags"], sets["performers includes_all"])))
			check("NOT (has_markers OR (tags AND NOT performers includes_all))", want, f)
		}
		// tags includes_all AND (has_markers OR performers)
		{
			f := and(cond("tags includes_all"), cond("has_markers"))
			f.And.Or = cond("performers").filter()
			want := setAnd(sets["tags includes_all"], setOr(sets["has_markers"], sets["performers"]))
			check("tags includes_all AND (has_markers OR performers)", want, f)
		}
		// performers AND (studios OR galleries)
		{
			f := and(cond("performers"), cond("studios"))
			f.And.Or = cond("galleries").filter()
			want := setAnd(sets["performers"], setOr(sets["studios"], sets["galleries"]))
			check("performers AND (studios OR galleries)", want, f)
		}
		// NOT (performers AND NOT tags)
		{
			f := notOnly(cond("performers"))
			f.Not.Not = cond("tags").filter()
			want := setMinus(all, setMinus(sets["performers"], sets["tags"]))
			check("NOT (performers AND NOT tags)", want, f)
		}
		// duplicated phash OR (none AND has_markers)
		{
			f := or(cond("duplicated phash"), none)
			f.Or.And = cond("has_markers").filter()
			check("duplicated phash OR (none AND has_markers)", sets["duplicated phash"], f)
		}
		// the same table joined at two levels:
		// performers includes_all AND NOT (tags OR performers)
		{
			f := not(cond("performers includes_all"), cond("tags"))
			f.Not.Or = cond("performers").filter()
			want := setMinus(sets["performers includes_all"], setOr(sets["tags"], sets["performers"]))
			check("performers includes_all AND NOT (tags OR performers)", want, f)
		}

		return nil
	})
}
