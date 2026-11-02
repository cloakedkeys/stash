package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antchfx/htmlquery"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// a trimmed copy of what https://literotica.com/api/3/stories/<name> sent on
// November 1, 2026
const literoticaStoryJSON = `{
  "meta": {"pages_count": 5},
  "submission": {
    "authorname": "Voboy",
    "category_info": {"type": "story", "pageUrl": "mature-sex"},
    "date_approve": "10/02/2026",
    "description": "The pool party was risky, but the Halloween party... well.",
    "id": 4542868,
    "newlanguage": 1,
    "tags": [
      {"id": 1, "tag": "pool", "is_banned": false},
      {"id": 2, "tag": "hot tub", "is_banned": false},
      {"id": 3, "tag": "halloween", "is_banned": false}
    ],
    "title": "Parallel Tracks",
    "url": "parallel-tracks"
  },
  "pageText": "The first page of the story."
}`

func loadLiteroticaDefinition(t *testing.T) *Definition {
	t.Helper()
	c, err := loadConfigFromYAML(LiteroticaScraperID, strings.NewReader(literoticaScraperConfig))
	require.NoError(t, err)
	return c
}

func TestLiteroticaStoryURL(t *testing.T) {
	c := loadLiteroticaDefinition(t)
	require.Len(t, c.StoryByURL, 1)
	byURL := *c.StoryByURL[0]

	const api = "https://literotica.com/api/3/stories/parallel-tracks"
	for _, u := range []string{
		"https://www.literotica.com/s/parallel-tracks",
		"https://literotica.com/s/parallel-tracks",
		"http://www.literotica.com/s/parallel-tracks?page=3",
		"https://www.literotica.com/s/parallel-tracks#comments",
		// language subdomains: the same API serves their stories
		"https://german.literotica.com/s/parallel-tracks",
		"https://french.literotica.com/s/parallel-tracks?page=2",
	} {
		assert.True(t, byURL.matchesURL(u), u)
		assert.Equal(t, api, replaceURL(u, byURL), u)
	}

	assert.False(t, byURL.matchesURL("https://www.literotica.com/stories/memberpage.php?uid=1"))

	// other addresses that contain Literotica's don't turn the scraper on
	for _, u := range []string{
		"http://127.0.0.1:19960/?x=literotica.com/s/test",
		"http://192.168.1.1/literotica.com/s/test",
		"https://notliterotica.com/s/test",
	} {
		assert.False(t, byURL.matchesURL(u), u)
	}

	// whatever the link, the request only ever goes to Literotica's API
	apiPrefix := "https://literotica.com/api/3/stories/"
	for _, u := range []string{
		"https://evil.example/?u=https://www.literotica.com/s/test",
		"http://127.0.0.1:19960/?x=literotica.com/s/test",
		"https://www.literotica.com/s/../../../admin?x=1",
		"https://www.literotica.com/s/a%2F..%2Fb",
	} {
		got := replaceURL(u, byURL)
		require.True(t, strings.HasPrefix(got, apiPrefix), "%s -> %s", u, got)
		assert.Regexp(t, `^[A-Za-z0-9_-]*$`, strings.TrimPrefix(got, apiPrefix), u)
	}
}

func TestLiteroticaStoryFields(t *testing.T) {
	c := loadLiteroticaDefinition(t)
	s, err := c.JsonScrapers["storyScraper"].scrapeStory(context.Background(), &jsonQuery{doc: literoticaStoryJSON})
	require.NoError(t, err)
	require.NotNil(t, s)

	assert.Equal(t, "Parallel Tracks", *s.Title)
	assert.Equal(t, "Voboy", *s.Author)
	assert.Equal(t, "Literotica", *s.SourceWebsite)
	// the story keeps the link it was scraped from, so no link is returned
	assert.Empty(t, s.URLs)
	assert.Equal(t, "2026-10-02", *s.Date)
	assert.Equal(t, "The pool party was risky, but the Halloween party... well.", *s.Tagline)
	assert.Nil(t, s.Details)
	assert.Nil(t, s.Language)

	var tags []string
	for _, tag := range s.Tags {
		tags = append(tags, tag.Name)
	}
	assert.Equal(t, []string{"pool", "hot tub", "halloween"}, tags)
}

func TestLiteroticaSpec(t *testing.T) {
	c := loadLiteroticaDefinition(t)
	spec := c.spec()
	require.NotNil(t, spec.Story)
	assert.Equal(t, []ScrapeType{ScrapeTypeURL}, spec.Story.SupportedScrapes)
	assert.Equal(t, []string{"://literotica.com/s/", ".literotica.com/s/"}, spec.Story.Urls)
	assert.True(t, c.supports(ScrapeContentTypeStory))
	assert.True(t, c.matchesURL("https://www.literotica.com/s/parallel-tracks", ScrapeContentTypeStory))
	assert.False(t, c.matchesURL("https://www.literotica.com/s/parallel-tracks", ScrapeContentTypeScene))
	assert.Nil(t, spec.Scene)
}

// A story scraper in any scraper file gets its details and tags, and a page
// with nothing found gives no story.
func TestStoryScraperXPath(t *testing.T) {
	c, err := loadConfigFromYAML("test", strings.NewReader(`name: Test
storyByURL:
  - action: scrapeXPath
    url: [example.com/story/]
    scraper: s
xPathScrapers:
  s:
    story:
      Title: //h1
      Author: //span[@class="author"]
      Details: //div[@class="summary"]
      Language:
        fixed: English
      URLs: //link[@rel="canonical"]/@href
      Tags:
        Name: //a[@class="tag"]
`))
	require.NoError(t, err)

	const page = `<html><head><link rel="canonical" href="https://example.com/story/1"></head><body>
<h1>A Title</h1><span class="author">Someone</span><div class="summary">What happens.</div>
<a class="tag">one</a><a class="tag">two</a></body></html>`
	q := &xpathQuery{doc: mustParseHTML(t, page)}
	s, err := c.XPathScrapers["s"].scrapeStory(context.Background(), q)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "A Title", *s.Title)
	assert.Equal(t, "Someone", *s.Author)
	assert.Equal(t, "What happens.", *s.Details)
	assert.Equal(t, "English", *s.Language)
	assert.Equal(t, []string{"https://example.com/story/1"}, s.URLs)
	require.Len(t, s.Tags, 2)
	assert.Equal(t, "two", s.Tags[1].Name)

	// a page with none of the fields gives no story
	c2, err := loadConfigFromYAML("test", strings.NewReader(`name: Test
xPathScrapers:
  s:
    story:
      Title: //h1
`))
	require.NoError(t, err)
	none, err := c2.XPathScrapers["s"].scrapeStory(context.Background(), &xpathQuery{doc: mustParseHTML(t, "<html><body></body></html>")})
	require.NoError(t, err)
	assert.Nil(t, none)
}

func mustParseHTML(t *testing.T, s string) *html.Node {
	t.Helper()
	doc, err := htmlquery.Parse(strings.NewReader(s))
	require.NoError(t, err)
	return doc
}

// A story URL goes to the scraper whose storyByURL matches it, and its query
// URL is loaded. The Literotica scraper's mapping is used, with the API
// address swapped for a local server.
func TestStoryScrapeByURL(t *testing.T) {
	var asked string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(literoticaStoryJSON))
	}))
	defer ts.Close()

	yml := strings.Replace(literoticaScraperConfig, "https://literotica.com/api/3/stories/{url}", ts.URL+"/api/3/stories/{url}", 1)
	require.NotEqual(t, literoticaScraperConfig, yml)
	c, err := loadConfigFromYAML("test", strings.NewReader(yml))
	require.NoError(t, err)

	s := scraperFromDefinition(*c, mockGlobalConfig{})
	content, err := s.viaURL(context.Background(), ts.Client(), "https://www.literotica.com/s/parallel-tracks", ScrapeContentTypeStory)
	require.NoError(t, err)
	story, ok := content.(*models.ScrapedStory)
	require.True(t, ok, "%T", content)
	assert.Equal(t, "/api/3/stories/parallel-tracks", asked)
	assert.Equal(t, "Parallel Tracks", *story.Title)

	// other URLs and other kinds of object aren't scraped
	content, err = s.viaURL(context.Background(), ts.Client(), "https://example.com/s/x", ScrapeContentTypeStory)
	require.NoError(t, err)
	assert.Nil(t, content)
	assert.False(t, s.supportsURL("https://www.literotica.com/s/parallel-tracks", ScrapeContentTypeScene))
}
