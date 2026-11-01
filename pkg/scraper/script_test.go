package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run the test executable as a scraper so these tests need no external interpreter.
func TestScriptImageOutputProcess(t *testing.T) {
	output, ok := os.LookupEnv("STASH_TEST_IMAGE_SCRIPT_OUTPUT")
	if !ok {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	_, _ = io.WriteString(os.Stdout, output)
	os.Exit(0)
}

func TestScriptImageOutput(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	command := []string{exe, "-test.run=^TestScriptImageOutputProcess$"}
	s := scriptFragmentScraper{
		scriptScraper: scriptScraper{definition: Definition{path: filepath.Join(t.TempDir(), "scraper.yml")}},
		definition:    ByFragmentDefinition{ActionDefinition: ActionDefinition{Script: command}},
	}
	u := scriptURLScraper{
		scriptScraper: s.scriptScraper,
		definition:    ByURLDefinition{ActionDefinition: ActionDefinition{Script: command}},
	}

	for _, tt := range []struct {
		name    string
		output  string
		noMatch bool
		wantErr bool
	}{
		{name: "empty object", output: `{}`, noMatch: true},
		{name: "empty object with whitespace", output: " \n{ }\n", noMatch: true},
		{name: "null", output: `null`, noMatch: true},
		{name: "title", output: `{"title":"Image"}`},
		{name: "explicit empty title", output: `{"title":""}`},
		{name: "code", output: `{"code":"ABC"}`},
		{name: "details", output: `{"details":"Description"}`},
		{name: "photographer", output: `{"photographer":"Photographer"}`},
		{name: "date", output: `{"date":"2026-01-01"}`},
		{name: "urls", output: `{"urls":["https://example.com/image"]}`},
		{name: "studio", output: `{"studio":{"name":"Studio"}}`},
		{name: "tags", output: `{"tags":[{"name":"Tag"}]}`},
		{name: "performers", output: `{"performers":[{"name":"Performer"}]}`},
		{name: "no output", wantErr: true},
		{name: "invalid JSON", output: `invalid`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STASH_TEST_IMAGE_SCRIPT_OUTPUT", tt.output)
			for _, path := range []string{"image", "fragment", "url"} {
				t.Run(path, func(t *testing.T) {
					var got ScrapedContent
					var err error
					switch path {
					case "image":
						var image *models.ScrapedImage
						image, err = s.scrapeImageByImage(context.Background(), &models.Image{
							URLs:  models.NewRelatedStrings([]string{}),
							Files: models.NewRelatedFiles([]models.File{}),
						})
						if image != nil {
							got = image
						}
					case "fragment":
						got, err = s.scrapeByFragment(context.Background(), Input{Image: &models.ScrapedImageInput{}})
					case "url":
						got, err = u.scrapeByURL(context.Background(), "https://example.com/image", ScrapeContentTypeImage)
					}
					if tt.wantErr {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					if tt.noMatch {
						// Check the interface itself, including typed nil pointers.
						assert.True(t, got == nil, "expected no image, got %#v", got)
						return
					}
					var want models.ScrapedImage
					require.NoError(t, json.Unmarshal([]byte(tt.output), &want))
					assert.Equal(t, &want, got)
				})
			}
		})
	}
}

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
