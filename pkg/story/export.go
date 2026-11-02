package story

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/json"
	"github.com/stashapp/stash/pkg/models/jsonschema"
)

// ToBasicJSON converts a story into its JSON form, without its tags. Its URLs and
// files must be loaded.
func ToBasicJSON(s *models.Story) *jsonschema.Story {
	ret := &jsonschema.Story{
		Title:         s.Title,
		Author:        s.Author,
		SourceWebsite: s.SourceWebsite,
		Language:      s.Language,
		Details:       s.Details,
		Tagline:       s.Tagline,
		URLs:          s.URLs.List(),
		CreatedAt:     json.JSONTime{Time: s.CreatedAt},
		UpdatedAt:     json.JSONTime{Time: s.UpdatedAt},
	}
	if s.Date != nil {
		ret.Date = s.Date.String()
	}
	if s.Rating != nil {
		ret.Rating = *s.Rating
	}
	ret.Files = filePaths(s)
	return ret
}

// TextGetter gets the text stored with a story.
type TextGetter interface {
	GetText(ctx context.Context, id int) (*string, error)
}

// AddText adds the text stored with a story to its JSON. A story with a text file
// shows the file's text, so its stored text, if any, isn't exported. Its files must be
// loaded.
func AddText(ctx context.Context, r TextGetter, s *models.Story, j *jsonschema.Story) error {
	if len(s.Files.List()) > 0 {
		return nil
	}
	text, err := r.GetText(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("getting text: %w", err)
	}
	if text != nil {
		j.Text = *text
	}
	return nil
}

// filePaths lists the story's file paths with the primary file first, since an import
// makes the first file the primary one.
func filePaths(s *models.Story) []string {
	var ret []string
	files := s.Files.List()
	for _, f := range files {
		if s.PrimaryFileID != nil && f.Base().ID == *s.PrimaryFileID {
			ret = append(ret, f.Base().Path)
		}
	}
	for _, f := range files {
		if s.PrimaryFileID == nil || f.Base().ID != *s.PrimaryFileID {
			ret = append(ret, f.Base().Path)
		}
	}
	return ret
}
