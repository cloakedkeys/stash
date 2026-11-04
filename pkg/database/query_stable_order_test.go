//go:build integration

package database_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stableOrderToken is the name of every item these tests make, so that a
// search finds them and nothing else.
const stableOrderToken = "stableordertoken"

// stableOrderCaseName returns the token with the letters picked by i in
// capitals. Studio names must differ, so studios use these instead.
func stableOrderCaseName(i int) string {
	b := []byte(stableOrderToken)
	for bit := 0; bit < len(b); bit++ {
		if i&(1<<bit) != 0 {
			b[bit] = strings.ToUpper(string(b[bit]))[0]
		}
	}
	return string(b)
}

type stableOrderCase struct {
	name string
	// sorts are the sorts that rank every item the same. "" is no sort, or the
	// default sort of lists that have one.
	sorts []string
	// nameSorts are the sorts whose first key is the name or title.
	nameSorts []string
	// nameLast is whether every sort ends with the name or title.
	nameLast bool
	// idOrder is whether ties come back in a set id order on both databases.
	// Studio names must differ, and only PostgreSQL sorts names that differ
	// only in case as the same, so SQLite still ranks the studios by name.
	idOrder bool
	create  func(t *testing.T, ctx context.Context, i int) int
	query   func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int)
}

// newestFirst returns whether the items, all with the same name, come back
// newest first. That is the order SQLite gives once a sort reaches a name in
// ascending order, since its natural_ci collation never finds two names
// equal. A descending name, or a sort that never reaches a name, gives them
// oldest first.
func (c stableOrderCase) newestFirst(sort string, direction models.SortDirectionEnum) bool {
	for _, s := range c.nameSorts {
		if s == sort {
			return direction == models.SortDirectionEnumAsc
		}
	}
	return sort != "" && c.nameLast
}

func stableOrderCases() []stableOrderCase {
	q := stableOrderToken
	searched := func(find *models.FindFilterType) *models.FindFilterType {
		find.Q = &q
		return find
	}
	ids := func(n int, id func(i int) int) []int {
		ret := make([]int, n)
		for i := range ret {
			ret[i] = id(i)
		}
		return ret
	}
	// a time with no fraction of a second, so both databases keep it whole
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	var markerScene, markerTag int

	return []stableOrderCase{
		{
			name:      "scene",
			nameSorts: []string{"title"},
			nameLast:  true,
			sorts:     []string{"", "title", "date", "rating", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := models.NewScene()
				o.Title = stableOrderToken
				o.CreatedAt = createdAt
				o.UpdatedAt = createdAt
				require.NoError(t, db.Scene().Create(ctx, &o, nil))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: searched(find), Count: true}})
				require.NoError(t, err)
				return r.IDs, r.Count
			},
		},
		{
			name:      "audio",
			nameSorts: []string{"title"},
			nameLast:  true,
			sorts:     []string{"", "title", "date", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.Audio{Title: stableOrderToken, CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.Audio().Create(ctx, o, nil))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, err := db.Audio().Query(ctx, models.AudioQueryOptions{QueryOptions: models.QueryOptions{FindFilter: searched(find), Count: true}})
				require.NoError(t, err)
				return r.IDs, r.Count
			},
		},
		{
			name:      "image",
			nameSorts: []string{"title"},
			nameLast:  true,
			sorts:     []string{"", "title", "date", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.Image{Title: stableOrderToken, CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: o}))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, err := db.Image().Query(ctx, models.ImageQueryOptions{QueryOptions: models.QueryOptions{FindFilter: searched(find), Count: true}})
				require.NoError(t, err)
				return r.IDs, r.Count
			},
		},
		{
			name:      "gallery",
			nameSorts: []string{"title"},
			nameLast:  true,
			sorts:     []string{"", "title", "date", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.Gallery{Title: stableOrderToken, CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: o}))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, count, err := db.Gallery().Query(ctx, nil, searched(find))
				require.NoError(t, err)
				return ids(len(r), func(i int) int { return r[i].ID }), count
			},
		},
		{
			name:      "performer",
			nameSorts: []string{"", "name"},
			nameLast:  true,
			sorts:     []string{"", "name", "birthdate", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := models.NewPerformer()
				o.Name = stableOrderToken
				o.Disambiguation = fmt.Sprintf("%d", i)
				o.CreatedAt = createdAt
				o.UpdatedAt = createdAt
				require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &o}))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, count, err := db.Performer().Query(ctx, nil, searched(find))
				require.NoError(t, err)
				return ids(len(r), func(i int) int { return r[i].ID }), count
			},
		},
		{
			name:  "studio",
			sorts: []string{"", "name", "created_at"},
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.Studio{Name: stableOrderCaseName(i), CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: o}))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, count, err := db.Studio().Query(ctx, nil, searched(find))
				require.NoError(t, err)
				return ids(len(r), func(i int) int { return r[i].ID }), count
			},
		},
		{
			name:      "tag",
			nameSorts: []string{"", "name"},
			nameLast:  true,
			sorts:     []string{"", "name", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.Tag{Name: stableOrderToken, CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: o}))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, count, err := db.Tag().Query(ctx, nil, searched(find))
				require.NoError(t, err)
				return ids(len(r), func(i int) int { return r[i].ID }), count
			},
		},
		{
			name:      "group",
			nameSorts: []string{"", "name"},
			nameLast:  true,
			sorts:     []string{"", "name", "date", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.Group{Name: stableOrderToken, CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.Group().Create(ctx, o))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, count, err := db.Group().Query(ctx, nil, searched(find))
				require.NoError(t, err)
				return ids(len(r), func(i int) int { return r[i].ID }), count
			},
		},
		{
			name:      "story",
			nameSorts: []string{"", "title"},
			sorts:     []string{"", "title", "date", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := models.NewStory()
				o.Title = stableOrderToken
				o.CreatedAt = createdAt
				o.UpdatedAt = createdAt
				require.NoError(t, db.Story().Create(ctx, &o, nil))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, err := db.Story().Query(ctx, models.StoryQueryOptions{QueryOptions: models.QueryOptions{FindFilter: searched(find), Count: true}})
				require.NoError(t, err)
				return r.IDs, r.Count
			},
		},
		{
			name:      "scene marker",
			nameSorts: []string{"", "title"},
			sorts:     []string{"", "title", "seconds", "created_at"},
			idOrder:   true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				if i == 0 {
					s := models.NewScene()
					require.NoError(t, db.Scene().Create(ctx, &s, nil))
					markerScene = s.ID
					tag := &models.Tag{Name: "stable order marker tag"}
					require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: tag}))
					markerTag = tag.ID
				}
				o := &models.SceneMarker{Title: stableOrderToken, SceneID: markerScene, PrimaryTagID: markerTag, Seconds: 1, CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.SceneMarker().Create(ctx, o))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, count, err := db.SceneMarker().Query(ctx, nil, searched(find))
				require.NoError(t, err)
				return ids(len(r), func(i int) int { return r[i].ID }), count
			},
		},
		{
			name:    "folder",
			sorts:   []string{"", "created_at", "updated_at"},
			idOrder: true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.Folder{Path: fmt.Sprintf("/%s/%d", stableOrderToken, i), CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.Folder().Create(ctx, o))
				return int(o.ID)
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, err := db.Folder().Query(ctx, models.FolderQueryOptions{QueryOptions: models.QueryOptions{FindFilter: searched(find), Count: true}})
				require.NoError(t, err)
				return ids(len(r.IDs), func(i int) int { return int(r.IDs[i]) }), r.Count
			},
		},
		{
			name:    "file",
			sorts:   []string{"", "created_at", "updated_at"},
			idOrder: true,
			create: func(t *testing.T, ctx context.Context, i int) int {
				o := &models.BaseFile{Basename: fmt.Sprintf("%s_%d", stableOrderToken, i), ParentFolderID: folderIDs[folderIdxWithFiles], CreatedAt: createdAt, UpdatedAt: createdAt}
				require.NoError(t, db.File().Create(ctx, o))
				return int(o.ID)
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) ([]int, int) {
				r, err := db.File().Query(ctx, models.FileQueryOptions{QueryOptions: models.QueryOptions{FindFilter: searched(find), Count: true}})
				require.NoError(t, err)
				return ids(len(r.IDs), func(i int) int { return int(r.IDs[i]) }), r.Count
			},
		},
	}
}

// Items that a list's sort ranks the same, or every item when there is no sort,
// must come back in a set id order: with a sort, the one SQLite gives, and
// with no sort, id order (SQLite's depends on which index it reads, and for
// these items it is id order too). Paging through the
// list then gives every item exactly once, in the same order every time.
func TestQueryStableOrder(t *testing.T) {
	const items = 7
	const perPage = 2

	for _, tt := range stableOrderCases() {
		for _, sort := range tt.sorts {
			for _, direction := range []models.SortDirectionEnum{models.SortDirectionEnumAsc, models.SortDirectionEnumDesc} {
				if sort == "" && direction == models.SortDirectionEnumDesc {
					continue // with no sort, the direction does nothing
				}
				runWithRollbackTxn(t, fmt.Sprintf("%s sort %q %s", tt.name, sort, direction), func(t *testing.T, ctx context.Context) {
					var created []int
					for i := 0; i < items; i++ {
						created = append(created, tt.create(t, ctx, i))
					}

					find := func(page, perPage int) *models.FindFilterType {
						f := &models.FindFilterType{Page: &page, PerPage: &perPage}
						if sort != "" {
							s, d := sort, direction
							f.Sort, f.Direction = &s, &d
						}
						return f
					}

					want := slices.Clone(created)
					if tt.newestFirst(sort, direction) {
						slices.Reverse(want)
					}

					var first []int
					for run := 0; run < 3; run++ {
						all, count := tt.query(t, ctx, find(1, models.PerPageAll))
						require.Equal(t, items, count, "count")
						assert.ElementsMatch(t, created, all, "all results, run %d", run)
						if tt.idOrder {
							assert.Equal(t, want, all, "all results in the set order, run %d", run)
						}
						if run == 0 {
							first = all
						}
						assert.Equal(t, first, all, "same order as run 0, run %d", run)

						var paged []int
						for page := 1; (page-1)*perPage < items; page++ {
							ids, _ := tt.query(t, ctx, find(page, perPage))
							paged = append(paged, ids...)
						}
						assert.Equal(t, all, paged, "paged results, run %d", run)
					}
				})
			}
		}
	}
}

// A seeded random sort must stay random, and the same seed must give the same
// order on every page and every run.
func TestQueryStableOrderRandom(t *testing.T) {
	const items = 12
	const perPage = 5

	for _, tt := range stableOrderCases() {
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			var created []int
			for i := 0; i < items; i++ {
				created = append(created, tt.create(t, ctx, i))
			}

			find := func(seed string, page, perPage int) *models.FindFilterType {
				sort := "random_" + seed
				return &models.FindFilterType{Sort: &sort, Page: &page, PerPage: &perPage}
			}

			orders := make(map[string][]int)
			for _, seed := range []string{"1234", "98765"} {
				all, count := tt.query(t, ctx, find(seed, 1, models.PerPageAll))
				require.Equal(t, items, count, "count")
				assert.ElementsMatch(t, created, all, "seed %s", seed)
				orders[seed] = all

				for run := 0; run < 2; run++ {
					var paged []int
					for page := 1; (page-1)*perPage < items; page++ {
						ids, _ := tt.query(t, ctx, find(seed, page, perPage))
						paged = append(paged, ids...)
					}
					assert.Equal(t, all, paged, "seed %s paged, run %d", seed, run)
				}
			}

			assert.NotEqual(t, created, orders["1234"], "a random sort is not id order")
			assert.NotEqual(t, orders["1234"], orders["98765"], "seeds give different orders")
		})
	}
}

// Tags that a sort ranks the same come back in order of their sort name, or
// their name when they have none, as on SQLite.
func TestQueryTagTiesBySortName(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, sort := range []string{"created_at", "scenes_count", "updated_at"} {
		for _, direction := range []models.SortDirectionEnum{models.SortDirectionEnumAsc, models.SortDirectionEnumDesc} {
			runWithRollbackTxn(t, fmt.Sprintf("%s %s", sort, direction), func(t *testing.T, ctx context.Context) {
				create := func(name, sortName string) int {
					o := &models.Tag{Name: name, SortName: sortName, CreatedAt: createdAt, UpdatedAt: createdAt}
					require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: o}))
					return o.ID
				}
				// by name alone, the order would be second, third, first
				first := create("tagsortkey 3", "1")
				second := create("tagsortkey 1", "3")
				third := create("tagsortkey 2", "")

				q := "tagsortkey"
				s, d := sort, direction
				perPage := models.PerPageAll
				tags, _, err := db.Tag().Query(ctx, nil, &models.FindFilterType{Q: &q, Sort: &s, Direction: &d, PerPage: &perPage})
				require.NoError(t, err)
				ids := make([]int, len(tags))
				for i, tag := range tags {
					ids[i] = tag.ID
				}
				assert.Equal(t, []int{first, second, third}, ids)
			})
		}
	}
}

// Sorted by studio, items with the same title come back as on SQLite. With a
// studio, the studio name decides: newest first ascending, oldest first
// descending. With no studio, SQLite moves on to the ascending title, so they
// come back newest first either way.
func TestQueryStudioSortTies(t *testing.T) {
	tests := []struct {
		name   string
		create func(t *testing.T, ctx context.Context, studioID *int) int
		query  func(t *testing.T, ctx context.Context, find *models.FindFilterType) []int
	}{
		{
			name: "scene",
			create: func(t *testing.T, ctx context.Context, studioID *int) int {
				o := models.NewScene()
				o.Title = stableOrderToken
				o.StudioID = studioID
				require.NoError(t, db.Scene().Create(ctx, &o, nil))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) []int {
				r, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: find}})
				require.NoError(t, err)
				return r.IDs
			},
		},
		{
			name: "audio",
			create: func(t *testing.T, ctx context.Context, studioID *int) int {
				o := &models.Audio{Title: stableOrderToken, StudioID: studioID}
				require.NoError(t, db.Audio().Create(ctx, o, nil))
				return o.ID
			},
			query: func(t *testing.T, ctx context.Context, find *models.FindFilterType) []int {
				r, err := db.Audio().Query(ctx, models.AudioQueryOptions{QueryOptions: models.QueryOptions{FindFilter: find}})
				require.NoError(t, err)
				return r.IDs
			},
		},
	}

	// only returns the ids in want, in the order of got
	only := func(got, want []int) []int {
		var ret []int
		for _, id := range got {
			if slices.Contains(want, id) {
				ret = append(ret, id)
			}
		}
		return ret
	}

	for _, tt := range tests {
		for _, direction := range []models.SortDirectionEnum{models.SortDirectionEnumAsc, models.SortDirectionEnumDesc} {
			runWithRollbackTxn(t, tt.name+" "+direction.String(), func(t *testing.T, ctx context.Context) {
				studio := &models.Studio{Name: stableOrderToken + " studio"}
				require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: studio}))

				var withStudio, noStudio []int
				for i := 0; i < 4; i++ {
					withStudio = append(withStudio, tt.create(t, ctx, &studio.ID))
					noStudio = append(noStudio, tt.create(t, ctx, nil))
				}

				q, sort, d := stableOrderToken, "studio", direction
				perPage := models.PerPageAll
				got := tt.query(t, ctx, &models.FindFilterType{Q: &q, Sort: &sort, Direction: &d, PerPage: &perPage})
				require.Len(t, got, 8)

				wantStudio := slices.Clone(withStudio)
				if direction == models.SortDirectionEnumAsc {
					slices.Reverse(wantStudio)
				}
				wantNone := slices.Clone(noStudio)
				slices.Reverse(wantNone)

				assert.Equal(t, wantStudio, only(got, withStudio), "with a studio")
				assert.Equal(t, wantNone, only(got, noStudio), "with no studio")
			})
		}
	}
}
