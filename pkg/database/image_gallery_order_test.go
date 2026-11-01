//go:build integration

package database_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageGalleryOrderSharedFiles(t *testing.T) {
	for _, kind := range []string{"zip", "folder", "manual"} {
		runWithRollbackTxn(t, kind, func(t *testing.T, ctx context.Context) {
			now := time.Now()
			newFile := func(folder models.FolderID, name string, zip *models.FileID) models.FileID {
				f := &models.BaseFile{
					Basename: name, ParentFolderID: folder,
					DirEntry:  models.DirEntry{ZipFileID: zip, ModTime: now},
					CreatedAt: now, UpdatedAt: now,
				}
				require.NoError(t, db.File().Create(ctx, f))
				return f.ID
			}
			var galleries []models.Gallery
			var folders []models.FolderID
			var zips []*models.FileID
			for _, name := range []string{"gallery_01", "gallery_99"} {
				folder := &models.Folder{Path: filepath.Join("gallery_order", name)}
				require.NoError(t, db.Folder().Create(ctx, folder))
				folders = append(folders, folder.ID)
				g := models.NewGallery()
				input := models.CreateGalleryInput{Gallery: &g}
				var zip *models.FileID
				if kind == "zip" {
					id := newFile(folderIDs[folderIdxWithFiles], name+".zip", nil)
					zip = &id
					input.FileIDs = []models.FileID{id}
				} else if kind == "folder" {
					g.FolderID = &folder.ID
				}
				require.NoError(t, db.Gallery().Create(ctx, &input))
				galleries = append(galleries, g)
				zips = append(zips, zip)
			}
			createImage := func(number, gallery int) models.Image {
				id := newFile(folders[gallery], fmt.Sprintf("%03d.jpg", number), zips[gallery])
				i := models.NewImage()
				i.GalleryIDs = models.NewRelatedIDs([]int{galleries[gallery].ID})
				require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &i, FileIDs: []models.FileID{id}}))
				return i
			}
			// The other copy sorts before 001 by both path and filename.
			shared := createImage(0, 0)
			first := createImage(1, 1)
			second := createImage(2, 1)
			thirdFile := newFile(folders[1], "003.jpg", zips[1])
			require.NoError(t, db.Image().AddFileID(ctx, shared.ID, thirdFile))
			_, err := db.Image().UpdatePartial(ctx, shared.ID, models.ImagePartial{GalleryIDs: &models.UpdateIDs{
				IDs: []int{galleries[1].ID}, Mode: models.RelationshipUpdateModeAdd,
			}})
			require.NoError(t, err)

			// The same image can have two files in one gallery (for example,
			// repeated pictures in a ZIP). It must still occupy just one index.
			duplicateFile := newFile(folders[1], "004.jpg", zips[1])
			require.NoError(t, db.Image().AddFileID(ctx, shared.ID, duplicateFile))

			want := []int{first.ID, second.ID, shared.ID}
			for _, sort := range []string{"path", "title"} {
				for _, direction := range []models.SortDirectionEnum{models.SortDirectionEnumAsc, models.SortDirectionEnumDesc} {
					t.Run(sort+"/"+string(direction), func(t *testing.T) {
						got, err := image.FindByGalleryID(ctx, db.Image(), galleries[1].ID, sort, direction)
						require.NoError(t, err)
						ids := make([]int, len(got))
						for n, i := range got {
							ids[n] = i.ID
						}
						if kind == "manual" {
							// There is no gallery-local filename to prefer.
							assert.ElementsMatch(t, want, ids)
							return
						}
						expected := append([]int(nil), want...)
						if direction == models.SortDirectionEnumDesc {
							expected = []int{shared.ID, second.ID, first.ID}
						}
						assert.Equal(t, expected, ids)
					})
				}
			}
			count, err := db.Image().CountByGalleryID(ctx, galleries[1].ID)
			require.NoError(t, err)
			assert.Equal(t, len(want), count)
			sort, perPage := "path", models.PerPageAll
			got, count, err := queryImagesWithCount(ctx, db.Image(), &models.ImageFilterType{
				Galleries: &models.MultiCriterionInput{
					Value: []string{strconv.Itoa(galleries[1].ID)}, Modifier: models.CriterionModifierIncludes,
				},
			}, &models.FindFilterType{Sort: &sort, PerPage: &perPage})
			require.NoError(t, err)
			assert.Len(t, got, len(want))
			assert.Equal(t, len(want), count)
			cover, err := image.FindGalleryCover(ctx, db.Image(), galleries[1].ID, `cover\.jpg$`)
			require.NoError(t, err)
			require.NotNil(t, cover)
			if kind == "manual" {
				assert.Contains(t, want, cover.ID)
			} else {
				assert.Equal(t, first.ID, cover.ID)
			}

			// The gallery index endpoint must agree with the image list.
			if kind != "manual" {
				got, err := db.Image().FindByGalleryID(ctx, galleries[1].ID)
				require.NoError(t, err)
				require.Len(t, got, len(want))
				for n, id := range want {
					assert.Equal(t, id, got[n].ID)
				}
				for n, id := range want {
					got, err := db.Image().FindByGalleryIDIndex(ctx, galleries[1].ID, uint(n))
					require.NoError(t, err)
					require.NotNil(t, got, strconv.Itoa(n))
					assert.Equal(t, id, got.ID)
				}
				gotImage, err := db.Image().FindByGalleryIDIndex(ctx, galleries[1].ID, uint(len(want)))
				require.NoError(t, err)
				assert.Nil(t, gotImage)
			}

			// Adding an image from elsewhere, or one without files, must not
			// remove it from a folder/ZIP gallery's results.
			remoteOnly := createImage(8, 0)
			_, err = db.Image().UpdatePartial(ctx, remoteOnly.ID, models.ImagePartial{GalleryIDs: &models.UpdateIDs{
				IDs: []int{galleries[1].ID}, Mode: models.RelationshipUpdateModeAdd,
			}})
			require.NoError(t, err)
			fileless := models.NewImage()
			fileless.GalleryIDs = models.NewRelatedIDs([]int{galleries[1].ID})
			require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &fileless}))
			got, err = image.Query(ctx, db.Image(), &models.ImageFilterType{
				Galleries: &models.MultiCriterionInput{
					Value: []string{strconv.Itoa(galleries[1].ID)}, Modifier: models.CriterionModifierIncludesAll,
				},
			}, &models.FindFilterType{Sort: &sort, PerPage: &perPage})
			require.NoError(t, err)
			var ids []int
			for _, i := range got {
				ids = append(ids, i.ID)
			}
			assert.ElementsMatch(t, append(want, remoteOnly.ID, fileless.ID), ids)
		})
	}
}

func TestImageGalleryFindAll(t *testing.T) {
	runWithRollbackTxn(t, "larger than default page", func(t *testing.T, ctx context.Context) {
		gallery := models.NewGallery()
		require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &gallery}))
		var want []int
		for n := 0; n < (models.FindFilterType{}).GetPageSize()+1; n++ {
			i := models.NewImage()
			i.GalleryIDs = models.NewRelatedIDs([]int{gallery.ID})
			require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &i}))
			want = append(want, i.ID)
		}
		got, err := db.Image().FindByGalleryID(ctx, gallery.ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, want, imagesToIDs(got))
	})
}
