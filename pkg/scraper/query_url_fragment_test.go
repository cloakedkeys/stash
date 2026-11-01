package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

type queryURLTransport func(*http.Request) (*http.Response, error)

func (f queryURLTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFragmentQueryURL(t *testing.T) {
	for _, action := range []string{"scrapeXPath", "scrapeJson"} {
		for _, kind := range []string{"scene", "gallery", "image", "queryScene"} {
			for _, tc := range []struct {
				name, template     string
				urls               []string
				wantURL, wantError string
			}{
				{"missing url", "{url}", nil, "", "missing fields for queryURL: url"},
				{"no match", "{url}", []string{"https://other.example/"}, "", "no matching URLs found for this scraper"},
				{"later match", "{url}", []string{"https://other.example/", "https://studio.example/update/1", "https://studio.example/update/2"}, "https://studio.example/update/1", ""},
				{"missing title", "https://studio.example/search?q={title}", nil, "", "missing fields for queryURL: title"},
				{"literal url", "https://studio.example/search", nil, "https://studio.example/search", ""},
				{"literal JSON", `https://studio.example/api?filter={"id":1}`, nil, "https://studio.example/api?filter={%22id%22:1}", ""},
				{"literal braces", "https://studio.example/api?filter={foo}", nil, "https://studio.example/api?filter={foo}", ""},
				{"literal JSON with missing title", `https://studio.example/api?filter={"id":1}&title={title}`, nil, "", "missing fields for queryURL: title"},
				{"missing file fields", "https://studio.example/search?hash={checksum}&name={filename}&hash2={checksum}", nil, "", "missing fields for queryURL: checksum, filename"},
			} {
				t.Run(action+"/"+kind+"/"+tc.name, func(t *testing.T) {
					if tc.urls == nil {
						tc.urls = []string{}
					}
					if kind == "scene" && tc.name == "missing file fields" {
						tc.template += "&oshash={oshash}&path={path}&phash={phash}"
						tc.wantError = "missing fields for queryURL: checksum, filename, oshash, path, phash"
					}
					mappedKey := "xPathScrapers"
					if action == "scrapeJson" {
						mappedKey = "jsonScrapers"
					}
					definition, err := loadConfigFromYAML("test", strings.NewReader(fmt.Sprintf(`
name: Test
%s:
  test:
    scene:
      Title:
        fixed: Example
    gallery:
      Title:
        fixed: Example
    image:
      Title:
        fixed: Example
`, mappedKey)))
					if err != nil {
						t.Fatal(err)
					}
					fragment := &ByFragmentDefinition{ActionDefinition: ActionDefinition{Action: scraperAction(action), Scraper: "test"}, QueryURL: tc.template}
					definition.SceneByFragment = fragment
					definition.SceneByQueryFragment = fragment
					definition.GalleryByFragment = fragment
					definition.ImageByFragment = fragment
					patterns := []*ByURLDefinition{{URL: []string{"studio.example/update"}}}
					definition.SceneByURL, definition.GalleryByURL, definition.ImageByURL = patterns, patterns, patterns
					var requests []string
					client := &http.Client{Transport: queryURLTransport(func(r *http.Request) (*http.Response, error) {
						requests = append(requests, r.URL.String())
						return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
					})}
					s := scraperFromDefinition(*definition, mockGlobalConfig{})
					switch kind {
					case "scene":
						_, err = s.viaScene(context.Background(), client, &models.Scene{Files: models.NewRelatedVideoFiles([]*models.VideoFile{}), URLs: models.NewRelatedStrings(tc.urls)})
					case "gallery":
						_, err = s.viaGallery(context.Background(), client, &models.Gallery{Files: models.NewRelatedFiles([]models.File{}), URLs: models.NewRelatedStrings(tc.urls)})
					case "image":
						_, err = s.viaImage(context.Background(), client, &models.Image{URLs: models.NewRelatedStrings(tc.urls)})
					case "queryScene":
						_, err = s.viaFragment(context.Background(), client, Input{Scene: &models.ScrapedSceneInput{URLs: tc.urls}})
					}
					if tc.wantError != "" {
						if err == nil || err.Error() != tc.wantError {
							t.Fatalf("error = %v, want %q", err, tc.wantError)
						}
						if len(requests) != 0 {
							t.Fatalf("requested URLs on validation failure: %v", requests)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						if len(requests) != 1 || requests[0] != tc.wantURL {
							t.Fatalf("requested URLs = %v, want [%s]", requests, tc.wantURL)
						}
					}
				})
			}
		}
	}
}
