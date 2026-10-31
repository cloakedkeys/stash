package scraper

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

func Test_imageInputFromImage_worksWithMultipleFiles(t *testing.T) {

	date, _ := models.ParseDate("2020-01-01")
	model := models.Image{
		ID:           1,
		Title:        "Test Image",
		URLs:         models.NewRelatedStrings([]string{"https://example.com/image.png"}),
		Date:         &date,
		Code:         "Code",
		Photographer: "Photographer",
		Files: models.NewRelatedFiles([]models.File{
			makeImageFile(1),
			makeImageFile(2),
		}),
	}

	input := imageInputFromImage(&model)

	assert.Equal(t, "1", input.ID)
	assert.Equal(t, "Test Image", input.Title)
	assert.Equal(t, "https://example.com/image.png", input.Urls[0])
	assert.Equal(t, "2020-01-01", *input.Date)
	assert.Equal(t, "Code", input.Code)
	assert.Equal(t, "Photographer", input.Photographer)
	assert.Equal(t, "/data/images/image_0001_.png", input.Files[0].Path)
	assert.Equal(t, "/data/images/image_0002_.png", input.Files[1].Path)
}

func getImageStringValue(index int, field string) string {
	return fmt.Sprintf("image_%04d_%s", index, field)
}

func makeImageFile(i int) *models.ImageFile {
	return &models.ImageFile{
		BaseFile: &models.BaseFile{
			Path:     "/data/images/" + getImageStringValue(i, ".png"),
			Basename: getImageStringValue(i, ".png"),
		},
		Height: 200,
		Width:  300,
	}
}

func Test_sceneInputFromScene_includesStudioAndPerformers(t *testing.T) {
	studioID := 3
	model := models.Scene{
		ID:           1,
		Title:        "Test Scene",
		Code:         "ABC-123",
		StudioID:     &studioID,
		PerformerIDs: models.NewRelatedIDs([]int{5, 7}),
		URLs:         models.NewRelatedStrings([]string{}),
		Files:        models.NewRelatedVideoFiles([]*models.VideoFile{}),
	}

	input := sceneInputFromScene(&model)

	assert.Equal(t, "ABC-123", input.Code)
	assert.Equal(t, "3", *input.StudioID)
	assert.Equal(t, []string{"5", "7"}, input.PerformerIDs)

	// left out of the JSON when the scene has none
	model.StudioID = nil
	model.PerformerIDs = models.NewRelatedIDs([]int{})
	b, err := json.Marshal(sceneInputFromScene(&model))
	assert.NoError(t, err)
	assert.NotContains(t, string(b), "studio_id")
	assert.NotContains(t, string(b), "performer_ids")
}
