package story

import (
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
