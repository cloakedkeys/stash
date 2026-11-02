package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// LiteroticaScraperID is the scraper ID for the built-in Literotica scraper
const LiteroticaScraperID = "builtin_literotica"

// literoticaScraperConfig reads a story's details from the JSON the
// Literotica story page itself loads. A story page is
// https://www.literotica.com/s/<name>, and its details are at
// https://literotica.com/api/3/stories/<name>.
const literoticaScraperConfig = `
name: Literotica
storyByURL:
  - action: scrapeJson
    # literotica.com and its subdomains, including the language ones such as
    # german.literotica.com, whose stories the same API serves
    url:
      - ://literotica.com/s/
      - .literotica.com/s/
    # always Literotica's address: the link is cut down to the story's name,
    # and anything left that can't be in a name is dropped, so a link that
    # only contains a Literotica address can't send the request elsewhere
    queryURL: "https://literotica.com/api/3/stories/{url}"
    queryURLReplace:
      url:
        - regex: ^https?://(?:[a-z]+\.)?literotica\.com/s/([^/?#]+).*$
          with: $1
        - regex: "[^A-Za-z0-9_-]"
          with: ""
    scraper: storyScraper

jsonScrapers:
  storyScraper:
    story:
      Title: submission.title
      Author: submission.authorname
      SourceWebsite:
        fixed: Literotica
      # no URL: the story keeps the link it was scraped from, whether that's
      # www.literotica.com or a language site such as german.literotica.com,
      # rather than gaining a second link to the same story
      Date:
        selector: submission.date_approve
        postProcess:
          - parseDate: 01/02/2006
      Tagline: submission.description
      Tags:
        Name: submission.tags.#.tag

# The story's text is fetched page by page in Go, below: each page is at
# https://literotica.com/api/3/stories/<name>?params={"contentPage":<n>}, and
# the first says how many pages there are in meta.pages_count.

# Last checked against the live site on November 2, 2026
`

func getLiteroticaScraper(globalConfig GlobalConfig) scraper {
	c, err := loadConfigFromYAML(LiteroticaScraperID, strings.NewReader(literoticaScraperConfig))
	if err != nil {
		logger.Fatalf("Error loading builtin literotica scraper: %s", err.Error())
	}

	return literoticaScraper{scraperFromDefinition(*c, globalConfig)}
}

// literoticaScraper gets a story's details with the scraper above, then its text. A
// story's text is split into pages, which the mapping can't follow, so every page
// is fetched here. If any page can't be fetched, nothing is scraped, rather than
// part of the story without saying so.
type literoticaScraper struct {
	definedScraper
}

func (s literoticaScraper) viaURL(ctx context.Context, client *http.Client, u string, ty ScrapeContentType) (ScrapedContent, error) {
	ret, err := s.definedScraper.viaURL(ctx, client, u, ty)
	if err != nil || ret == nil || ty != ScrapeContentTypeStory {
		return ret, err
	}
	story, ok := ret.(*models.ScrapedStory)
	if !ok {
		return ret, nil
	}

	for _, def := range s.config.StoryByURL {
		if def.matchesURL(u) {
			text, err := literoticaText(ctx, client, s.config, s.globalConf, replaceURL(u, *def))
			if err != nil {
				return nil, fmt.Errorf("couldn't get the story's text from Literotica, so nothing was scraped: %w", err)
			}
			story.Text = &text
			break
		}
	}

	return story, nil
}

// literoticaMaxPages is the most pages a story's text is fetched from, in case the
// site ever reports a page count that can't be right.
const literoticaMaxPages = 500

type literoticaPage struct {
	Meta struct {
		PagesCount int `json:"pages_count"`
	} `json:"meta"`
	PageText string `json:"pageText"`
}

// literoticaText fetches every page of a story's text from its API address, and
// joins them with a blank line between pages. The first page says how many there
// are.
func literoticaText(ctx context.Context, client *http.Client, def Definition, globalConfig GlobalConfig, apiURL string) (string, error) {
	var pages []string
	count := 1
	for n := 1; n <= count; n++ {
		page, err := literoticaPageText(ctx, client, def, globalConfig, apiURL, n)
		if err != nil {
			if n == 1 {
				return "", fmt.Errorf("the first page couldn't be loaded: %w", err)
			}
			return "", fmt.Errorf("page %d of %d couldn't be loaded: %w", n, count, err)
		}
		if n == 1 {
			count = max(page.Meta.PagesCount, 1)
			if count > literoticaMaxPages {
				return "", fmt.Errorf("the story says it has %d pages, more than the %d allowed", count, literoticaMaxPages)
			}
		}
		if strings.TrimSpace(page.PageText) == "" {
			return "", fmt.Errorf("page %d of %d has no text", n, count)
		}
		pages = append(pages, page.PageText)
	}
	return strings.Join(pages, "\n\n"), nil
}

func literoticaPageText(ctx context.Context, client *http.Client, def Definition, globalConfig GlobalConfig, apiURL string, n int) (*literoticaPage, error) {
	u := apiURL + "?params=" + url.QueryEscape(fmt.Sprintf(`{"contentPage":%d}`, n))
	r, err := loadURL(ctx, u, client, def, globalConfig, true)
	if err != nil {
		return nil, err
	}
	var page literoticaPage
	if err := json.NewDecoder(r).Decode(&page); err != nil {
		return nil, fmt.Errorf("reading the page: %w", err)
	}
	return &page, nil
}
