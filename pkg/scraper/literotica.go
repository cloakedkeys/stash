package scraper

import (
	"strings"

	"github.com/stashapp/stash/pkg/logger"
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
    # with the scheme, so language subdomains, which the API address below
    # doesn't cover, don't match
    url:
      - ://literotica.com/s/
      - ://www.literotica.com/s/
    # always Literotica's address: the link is cut down to the story's name,
    # and anything left that can't be in a name is dropped, so a link that
    # only contains a Literotica address can't send the request elsewhere
    queryURL: "https://literotica.com/api/3/stories/{url}"
    queryURLReplace:
      url:
        - regex: ^https?://(?:www\.)?literotica\.com/s/([^/?#]+).*$
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
      URL:
        selector: submission.url
        postProcess:
          - replace:
              - regex: ^
                with: https://www.literotica.com/s/
      Date:
        selector: submission.date_approve
        postProcess:
          - parseDate: 01/02/2006
      Tagline: submission.description
      Tags:
        Name: submission.tags.#.tag

# Last checked against the live site on November 1, 2026
`

func getLiteroticaScraper(globalConfig GlobalConfig) scraper {
	c, err := loadConfigFromYAML(LiteroticaScraperID, strings.NewReader(literoticaScraperConfig))
	if err != nil {
		logger.Fatalf("Error loading builtin literotica scraper: %s", err.Error())
	}

	return scraperFromDefinition(*c, globalConfig)
}
