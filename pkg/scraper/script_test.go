package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestRunScraperScriptReapsProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"empty", "malformed", "malformed-running", "valid", "unknown-field", "nonzero"} {
		t.Run(mode, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "pid")
			t.Setenv("STASH_TEST_SCRAPER_PID_FILE", pidFile)
			t.Setenv("STASH_TEST_SCRAPER_MODE", mode)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			s := scriptScraper{definition: Definition{path: filepath.Join(t.TempDir(), "scraper.yml")}}
			var out struct {
				Title string `json:"title"`
			}
			err := s.runScraperScript(ctx, []string{executable, "-test.run=^TestScraperScriptProcess$"}, "{}", &out)
			switch mode {
			case "valid", "unknown-field":
				assert.NoError(t, err)
				assert.Equal(t, "test", out.Title)
			case "nonzero":
				assert.ErrorIs(t, err, ErrScraperScript)
			default:
				assert.EqualError(t, err, "error running scraper script, check the logs for more information")
			}

			pidBytes, err := os.ReadFile(pidFile)
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(string(pidBytes))
			if err != nil {
				t.Fatal(err)
			}
			process, err := os.FindProcess(pid)
			if err != nil {
				// Windows cannot open an exited process once its handles are closed.
				return
			}
			defer func() { _ = process.Release() }()
			// On Unix, waiting again must fail because the scraper already reaped
			// the child. On Windows, an unreleased handle lets this wait succeed.
			if state, err := process.Wait(); err == nil {
				t.Errorf("scraper process was not reaped before returning: %v", state)
			}
		})
	}
}

func TestScraperScriptProcess(t *testing.T) {
	pidFile := os.Getenv("STASH_TEST_SCRAPER_PID_FILE")
	if pidFile == "" {
		return
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	mode := os.Getenv("STASH_TEST_SCRAPER_MODE")
	switch mode {
	case "empty":
	case "malformed", "malformed-running":
		fmt.Fprint(os.Stdout, "invalid json")
	case "unknown-field":
		fmt.Fprint(os.Stdout, `{"title":"test","extra":true}`)
	default:
		fmt.Fprint(os.Stdout, `{"title":"test"}`)
	}
	if mode == "malformed-running" {
		// A decode error must also clean up a child that has not exited yet.
		time.Sleep(time.Minute)
	}
	if mode == "nonzero" {
		os.Exit(1)
	}
	os.Exit(0)
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
