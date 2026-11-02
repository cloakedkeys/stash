package scraper

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Whichever scraper found a story's text, it comes out as plain text.
func TestPostScrapeStoryText(t *testing.T) {
	c := &postScraper{}
	text := func(in *string) *string {
		t.Helper()
		out, err := c.postScrapeStory(context.Background(), models.ScrapedStory{Text: in})
		require.NoError(t, err)
		return out.(models.ScrapedStory).Text
	}

	html := "<p>One &amp; two.</p><script>alert(1)</script><p>Three.</p>"
	got := text(&html)
	require.NotNil(t, got)
	assert.Equal(t, "One & two.\n\nThree.", *got)

	markupOnly := "<p> </p><br>"
	assert.Nil(t, text(&markupOnly), "text that's only markup is no text")
	assert.Nil(t, text(nil))
}
