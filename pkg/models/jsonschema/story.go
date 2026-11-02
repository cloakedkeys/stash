package jsonschema

import (
	"fmt"
	"os"
	"unicode/utf8"

	jsoniter "github.com/json-iterator/go"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models/json"
)

type Story struct {
	Title         string `json:"title,omitempty"`
	Author        string `json:"author,omitempty"`
	SourceWebsite string `json:"source_website,omitempty"`
	Date          string `json:"date,omitempty"`
	Language      string `json:"language,omitempty"`
	Details       string `json:"details,omitempty"`
	Tagline       string `json:"tagline,omitempty"`
	// the rating of the account that made the export
	Rating int      `json:"rating,omitempty"`
	URLs   []string `json:"urls,omitempty"`
	// paths of the story's files, the primary file first
	Files     []string      `json:"files,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
	CreatedAt json.JSONTime `json:"created_at,omitempty"`
	UpdatedAt json.JSONTime `json:"updated_at,omitempty"`
}

// maxStoryNameBytes keeps an export file name well under the 255 bytes most
// file systems allow. The id in the name keeps stories apart.
const maxStoryNameBytes = 100

func (s Story) Filename(basename string, id string) string {
	if len(basename) > maxStoryNameBytes {
		n := maxStoryNameBytes
		for n > 0 && !utf8.RuneStart(basename[n]) {
			n--
		}
		basename = basename[:n]
	}
	ret := fsutil.SanitiseBasename(basename)
	if ret != "" {
		ret += "."
	}
	return ret + id + ".json"
}

func LoadStoryFile(filePath string) (*Story, error) {
	var story Story
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.NewDecoder(file).Decode(&story); err != nil {
		return nil, err
	}
	return &story, nil
}

func SaveStoryFile(filePath string, story *Story) error {
	if story == nil {
		return fmt.Errorf("story must not be nil")
	}
	return marshalToFile(filePath, story)
}
