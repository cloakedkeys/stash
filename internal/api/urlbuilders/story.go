package urlbuilders

import (
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

type StoryURLBuilder struct {
	BaseURL string
	StoryID string
}

func NewStoryURLBuilder(baseURL string, story *models.Story) StoryURLBuilder {
	return StoryURLBuilder{
		BaseURL: baseURL,
		StoryID: strconv.Itoa(story.ID),
	}
}

// GetTextURL has no time in it, unlike cover URLs. The text can change on disk
// without the story changing, so the browser has to check it each time.
func (b StoryURLBuilder) GetTextURL() string {
	return b.BaseURL + "/story/" + b.StoryID + "/text"
}
