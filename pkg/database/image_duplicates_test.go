//go:build integration

package database_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestImageStoreFindDuplicates(t *testing.T) {
	for _, kind := range []string{"exact", "multiple files", "zip"} {
		runWithRollbackTxn(t, kind, func(t *testing.T, ctx context.Context) {
			gallery := models.NewGallery()
			galleryInput := &models.CreateGalleryInput{Gallery: &gallery}
			if kind == "zip" {
				galleryInput.FileIDs = []models.FileID{fileIDs[fileIdxZip]}
			}
			require.NoError(t, db.Gallery().Create(ctx, galleryInput))
			filter := &models.ImageFilterType{Galleries: &models.MultiCriterionInput{
				Value: []string{fmt.Sprint(gallery.ID)}, Modifier: models.CriterionModifierIncludes,
			}}
			newFile := func(name string, phash int64) models.FileID {
				f := &models.ImageFile{BaseFile: &models.BaseFile{
					Basename: name, ParentFolderID: folderIDs[folderIdxWithImageFiles],
					Fingerprints: models.Fingerprints{{Type: models.FingerprintTypePhash, Fingerprint: phash}},
				}}
				if kind == "zip" {
					f.ZipFileID = &fileIDs[fileIdxZip]
				}
				require.NoError(t, db.File().Create(ctx, f))
				return f.ID
			}
			newImage := func(ids ...models.FileID) models.Image {
				i := models.NewImage()
				i.GalleryIDs = models.NewRelatedIDs([]int{gallery.ID})
				require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &i, FileIDs: ids}))
				return i
			}
			first := newImage(newFile("duplicate-first.jpg", int64(1793)))
			newImage(newFile("duplicate-other.jpg", int64(0x7fffffffffffffff)))
			got, err := db.Image().FindDuplicates(ctx, 0, filter)
			require.NoError(t, err)
			require.Empty(t, got)
			if kind == "multiple files" {
				// Repeated copies on one image are not a duplicate group.
				require.NoError(t, db.Image().AddFileID(ctx, first.ID, newFile("duplicate-copy.jpg", int64(1793))))
				got, err = db.Image().FindDuplicates(ctx, 0, filter)
				require.NoError(t, err)
				require.Empty(t, got)
			}
			// Fingerprints of other types must not match a phash with the same value.
			unrelated := &models.ImageFile{BaseFile: &models.BaseFile{
				Basename: "duplicate-md5-only.jpg", ParentFolderID: folderIDs[folderIdxWithImageFiles],
				Fingerprints: models.Fingerprints{{Type: models.FingerprintTypeMD5, Fingerprint: "1793"}},
			}}
			require.NoError(t, db.File().Create(ctx, unrelated))
			newImage(unrelated.ID)
			missing := &models.ImageFile{BaseFile: &models.BaseFile{
				Basename: "duplicate-missing.jpg", ParentFolderID: folderIDs[folderIdxWithImageFiles],
			}}
			require.NoError(t, db.File().Create(ctx, missing))
			newImage(missing.ID)
			newImage()
			var second models.Image
			if kind == "multiple files" {
				// The matching file is secondary, not the primary file.
				second = newImage(newFile("duplicate-primary.jpg", int64(0x3333333333333333)), newFile("duplicate-secondary.jpg", int64(1793)))
			} else {
				second = newImage(newFile("duplicate-second.jpg", int64(1793)))
			}
			got, err = db.Image().FindDuplicates(ctx, 0, filter)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.ElementsMatch(t, []int{first.ID, second.ID}, imagesToIDs(got[0]))
			// The unfiltered query used by the page must also include this group.
			all, err := db.Image().FindDuplicates(ctx, 0, nil)
			require.NoError(t, err)
			found := false
			for _, group := range all {
				ids := imagesToIDs(group)
				if len(ids) == 2 && (ids[0] == first.ID || ids[1] == first.ID) &&
					(ids[0] == second.ID || ids[1] == second.ID) {
					found = true
				}
			}
			require.True(t, found)
			near := newImage(newFile("duplicate-near.jpg", int64(1793)^1))
			got, err = db.Image().FindDuplicates(ctx, 1, filter)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.ElementsMatch(t, []int{first.ID, second.ID, near.ID}, imagesToIDs(got[0]))
			got, err = db.Image().FindDuplicates(ctx, 0, filter)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.ElementsMatch(t, []int{first.ID, second.ID}, imagesToIDs(got[0]))
			// Filtering to one image must not include its match outside the filter.
			filter.ID = &models.IntCriterionInput{Value: first.ID, Modifier: models.CriterionModifierEquals}
			got, err = db.Image().FindDuplicates(ctx, 0, filter)
			require.NoError(t, err)
			require.Empty(t, got)
		})
	}
}
