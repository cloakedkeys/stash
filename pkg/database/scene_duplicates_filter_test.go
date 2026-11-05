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

// The duplicate checker sends the scene filter the user picked on the page.
// Each case below makes one group of three copies of a video; one copy differs
// in the way the filter looks at (organized, folder, tag, length). With the
// filter set, that copy must drop out of the group and the other two must
// still be found, for exact (distance 0) and near (distance 4) matches.
func TestSceneStore_FindDuplicatesSceneFilter(t *testing.T) {
	type dupeScene struct {
		organized bool
		skipped   bool // in the "skip" folder
		tagged    bool
		duration  float64
	}

	type filterCase struct {
		name string
		// phash shared by the group; far (in bits) from the fixture phashes and
		// from every other case, so groups never mix
		phash  int64
		scenes [3]dupeScene
		// indexes of the scenes the filter keeps
		want   []int
		filter func(tagID int) *models.SceneFilterType
	}

	normal := dupeScene{duration: 600}

	cases := []filterCase{
		{
			name:   "not organized",
			phash:  0x5A5A5A5A5A5A5A5A,
			scenes: [3]dupeScene{normal, normal, {organized: true, duration: 600}},
			want:   []int{0, 1},
			filter: func(int) *models.SceneFilterType {
				organized := false
				return &models.SceneFilterType{Organized: &organized}
			},
		},
		{
			name:   "path excludes folder",
			phash:  0x3C3C3C3C3C3C3C3C,
			scenes: [3]dupeScene{normal, normal, {skipped: true, duration: 600}},
			want:   []int{0, 1},
			filter: func(int) *models.SceneFilterType {
				return &models.SceneFilterType{Path: &models.StringCriterionInput{
					Value:    "dupfilter_skip",
					Modifier: models.CriterionModifierExcludes,
				}}
			},
		},
		{
			name:   "tag excludes",
			phash:  0x6969696969696969,
			scenes: [3]dupeScene{normal, normal, {tagged: true, duration: 600}},
			want:   []int{0, 1},
			filter: func(tagID int) *models.SceneFilterType {
				return &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{
					Value:    []string{strconv.Itoa(tagID)},
					Modifier: models.CriterionModifierExcludes,
				}}
			},
		},
		{
			name:  "tag includes",
			phash: 0x0F0F0F0F0F0F0F0F,
			scenes: [3]dupeScene{
				{tagged: true, duration: 600}, {tagged: true, duration: 600}, normal,
			},
			want: []int{0, 1},
			filter: func(tagID int) *models.SceneFilterType {
				return &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{
					Value:    []string{strconv.Itoa(tagID)},
					Modifier: models.CriterionModifierIncludes,
				}}
			},
		},
		{
			name:   "duration greater than",
			phash:  0x7171717171717171,
			scenes: [3]dupeScene{normal, normal, {duration: 5}},
			want:   []int{0, 1},
			filter: func(int) *models.SceneFilterType {
				return &models.SceneFilterType{Duration: &models.IntCriterionInput{
					Value:    10,
					Modifier: models.CriterionModifierGreaterThan,
				}}
			},
		},
	}

	for _, distance := range []int{0, 4} {
		for _, tc := range cases {
			name := fmt.Sprintf("%s/distance %d", tc.name, distance)
			runWithRollbackTxn(t, name, func(t *testing.T, ctx context.Context) {
				keep := &models.Folder{Path: "dupfilter_keep"}
				require.NoError(t, db.Folder().Create(ctx, keep))
				skip := &models.Folder{Path: "dupfilter_skip"}
				require.NoError(t, db.Folder().Create(ctx, skip))

				tag := models.NewTag()
				tag.Name = "dupfilter tag"
				require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tag}))

				ids := make([]int, len(tc.scenes))
				for i, s := range tc.scenes {
					phash := tc.phash
					if distance > 0 && i == 1 {
						// a near copy: one bit differs
						phash ^= 1
					}

					folder := keep
					if s.skipped {
						folder = skip
					}
					basename := fmt.Sprintf("dupfilter_%d.mp4", i)
					f := &models.VideoFile{
						BaseFile: &models.BaseFile{
							Path:           folder.Path + "/" + basename,
							Basename:       basename,
							ParentFolderID: folder.ID,
							Fingerprints: models.Fingerprints{
								{Type: models.FingerprintTypeMD5, Fingerprint: fmt.Sprintf("dupfilter_%x_%d", tc.phash, i)},
								{Type: models.FingerprintTypePhash, Fingerprint: phash},
							},
						},
						Duration: s.duration,
						Width:    1920,
						Height:   1080,
					}
					require.NoError(t, db.File().Create(ctx, f))

					scene := models.NewScene()
					scene.Title = basename
					scene.Organized = s.organized
					if s.tagged {
						scene.TagIDs = models.NewRelatedIDs([]int{tag.ID})
					}
					require.NoError(t, db.Scene().Create(ctx, &scene, []models.FileID{f.ID}))
					ids[i] = scene.ID
				}

				groupOf := func(filter *models.SceneFilterType) []int {
					groups, err := db.Scene().FindDuplicates(ctx, distance, -1, filter)
					require.NoError(t, err)

					var found [][]int
					for _, g := range groups {
						var groupIDs []int
						mine := false
						for _, s := range g {
							groupIDs = append(groupIDs, s.ID)
							for _, id := range ids {
								if s.ID == id {
									mine = true
								}
							}
						}
						if mine {
							sort.Ints(groupIDs)
							found = append(found, groupIDs)
						}
					}
					require.LessOrEqual(t, len(found), 1, "the copies should be in one group")
					if len(found) == 0 {
						return nil
					}
					return found[0]
				}

				all := append([]int(nil), ids...)
				sort.Ints(all)
				assert.Equal(t, all, groupOf(nil), "without a filter all three copies are found")

				var want []int
				for _, i := range tc.want {
					want = append(want, ids[i])
				}
				sort.Ints(want)
				assert.Equal(t, want, groupOf(tc.filter(tag.ID)), "the filter drops the odd copy")
			})
		}
	}
}
