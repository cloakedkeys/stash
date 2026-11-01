package scraper

import (
	"net/url"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

func TestConstructURL(t *testing.T) {
	tests := []struct {
		name     string
		template string
		params   queryURLParameters
		want     string
	}{
		{
			"plain values are unchanged",
			"https://example.com/api?hash={oshash}&limit=1",
			queryURLParameters{"oshash": "abc123"},
			"https://example.com/api?hash=abc123&limit=1",
		},
		{
			"spaces are encoded",
			"https://example.com/api?parse={filename}&limit=1",
			queryURLParameters{"filename": "My Video.mp4"},
			"https://example.com/api?parse=My%20Video.mp4&limit=1",
		},
		{
			"spaces in the path are encoded",
			"https://example.com/files/{filename}",
			queryURLParameters{"filename": "My Video.mp4"},
			"https://example.com/files/My%20Video.mp4",
		},
		{
			"non-ASCII characters are encoded as UTF-8",
			"https://example.com/api?parse={filename}",
			queryURLParameters{"filename": "Café.mp4"},
			"https://example.com/api?parse=Caf%C3%A9.mp4",
		},
		{
			"values already encoded by a replacement are not encoded again",
			"https://example.com/api?parse={filename}",
			queryURLParameters{"filename": "My%20Video+2.mp4"},
			"https://example.com/api?parse=My%20Video+2.mp4",
		},
		{
			"encoded characters in the template are kept",
			"https://example.com/search?title=Special%3ASearch&search={title}",
			queryURLParameters{"title": "Name"},
			"https://example.com/search?title=Special%3ASearch&search=Name",
		},
		{
			"a percent sign that is not an escape is encoded",
			"https://example.com/api?parse={filename}",
			queryURLParameters{"filename": "100% real.mp4"},
			"https://example.com/api?parse=100%25%20real.mp4",
		},
		{
			"a url replaced with a whole new url keeps its query string",
			"{url}",
			queryURLParameters{"url": "https://www.site.com/api/movie?name=title&date=20200101"},
			"https://www.site.com/api/movie?name=title&date=20200101",
		},
		{
			"the host is not changed",
			"https://bücher.example/api?q={title}",
			queryURLParameters{"title": "a b"},
			"https://bücher.example/api?q=a%20b",
		},
		{
			"a full path is encoded",
			"http://localhost/api/query?file={filename}&path={path}",
			queryURLParameters{"filename": "clip 1.mp4", "path": "/data/My Videos/clip 1.mp4"},
			"http://localhost/api/query?file=clip%201.mp4&path=/data/My%20Videos/clip%201.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.params.constructURL(tt.template)
			if got != tt.want {
				t.Errorf("constructURL(%q) = %q, want %q", tt.template, got, tt.want)
			}
			if _, err := url.Parse(got); err != nil {
				t.Errorf("constructURL(%q) = %q is not a valid URL: %v", tt.template, got, err)
			}
		})
	}
}

func TestQueryURLParametersFromScenePath(t *testing.T) {
	scenePath := "/data/My Videos/clip 1.mp4"
	scene := &models.Scene{
		Path:  scenePath,
		Files: models.NewRelatedVideoFiles([]*models.VideoFile{{BaseFile: &models.BaseFile{}}}),
		URLs:  models.NewRelatedStrings([]string{}),
	}

	params := queryURLParametersFromScene(scene)
	if params["path"] != scenePath {
		t.Errorf("path = %q, want %q", params["path"], scenePath)
	}
	if params["filename"] != "clip 1.mp4" {
		t.Errorf("filename = %q, want %q", params["filename"], "clip 1.mp4")
	}
}

func TestConstructFragmentURL(t *testing.T) {
	for _, tc := range []struct {
		name      string
		params    queryURLParameters
		urls      []string
		fragment  ByFragmentDefinition
		patterns  []*ByURLDefinition
		want      string
		wantError string
	}{
		{
			name:     "no URL patterns required",
			params:   queryURLParameters{},
			urls:     []string{"", "https://example.com/page"},
			fragment: ByFragmentDefinition{QueryURL: "{url}"},
			want:     "https://example.com/page",
		},
		{
			name:     "legacy fragment URL",
			params:   queryURLParametersFromScrapedScene(models.ScrapedSceneInput{URL: strPtr("https://example.com/page")}),
			fragment: ByFragmentDefinition{QueryURL: "{url}"},
			patterns: []*ByURLDefinition{{URL: []string{"example.com/page"}}},
			want:     "https://example.com/page",
		},
		{
			name:   "match before URL replacements",
			params: queryURLParameters{},
			urls:   []string{"https://other.example/", "https://example.com/page/1"},
			fragment: ByFragmentDefinition{
				QueryURL:             "{url}",
				QueryURLReplacements: queryURLReplacements{"url": {makeReplaceRegex("example.com/page", "api.example.com/item")}},
			},
			patterns: []*ByURLDefinition{{URL: []string{"example.com/page"}}},
			want:     "https://api.example.com/item/1",
		},
		{
			name:   "empty replacement value",
			params: queryURLParameters{"title": "Title"},
			fragment: ByFragmentDefinition{
				QueryURL:             "https://example.com/?q={title}",
				QueryURLReplacements: queryURLReplacements{"title": {makeReplaceRegex("Title", "")}},
			},
			wantError: "missing fields for queryURL: title",
		},
		{
			name:     "braces in a supplied value are not missing fields",
			params:   queryURLParameters{"title": "A {title}"},
			fragment: ByFragmentDefinition{QueryURL: "https://example.com/?q={title}"},
			want:     "https://example.com/?q=A%20{title}",
		},
		{
			name:      "explicitly empty fragment field",
			params:    queryURLParametersFromScrapedScene(models.ScrapedSceneInput{Title: strPtr("")}),
			fragment:  ByFragmentDefinition{QueryURL: "https://example.com/?q={title}"},
			wantError: "missing fields for queryURL: title",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := constructFragmentURL(tc.params, tc.urls, tc.fragment, Definition{SceneByURL: tc.patterns}, ScrapeContentTypeScene)
			if tc.wantError != "" {
				if err == nil || err.Error() != tc.wantError {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("URL = %q, error = %v; want %q", got, err, tc.want)
			}
		})
	}
}
