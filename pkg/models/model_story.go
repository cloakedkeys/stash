package models

import (
	"context"
	"time"
)

// Story stores metadata for an external text file.
type Story struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	SourceWebsite string `json:"source_website"`
	Date          *Date  `json:"date"`
	Language      string `json:"language"`
	Details       string `json:"details"`
	Tagline       string `json:"tagline"`
	// Rating is per user, expressed on the same 1-100 scale as audio.
	Rating        *int `json:"rating"`
	Files         RelatedFiles
	PrimaryFileID *FileID
	Path          string
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	URLs          RelatedStrings `json:"urls"`
	TagIDs        RelatedIDs     `json:"tag_ids"`
}

func NewStory() Story {
	now := time.Now()
	return Story{CreatedAt: now, UpdatedAt: now}
}

type StoryPartial struct {
	Title         OptionalString
	Author        OptionalString
	SourceWebsite OptionalString
	Date          OptionalDate
	Language      OptionalString
	Details       OptionalString
	Tagline       OptionalString
	Rating        OptionalInt
	CreatedAt     OptionalTime
	UpdatedAt     OptionalTime
	URLs          *UpdateStrings
	TagIDs        *UpdateIDs
	PrimaryFileID *FileID
}

func NewStoryPartial() StoryPartial {
	return StoryPartial{UpdatedAt: NewOptionalTime(time.Now())}
}

func (s *Story) LoadURLs(ctx context.Context, l URLLoader) error {
	return s.URLs.load(func() ([]string, error) { return l.GetURLs(ctx, s.ID) })
}
func (s *Story) LoadTagIDs(ctx context.Context, l TagIDLoader) error {
	return s.TagIDs.load(func() ([]int, error) { return l.GetTagIDs(ctx, s.ID) })
}
func (s *Story) LoadFiles(ctx context.Context, l FileLoader) error {
	return s.Files.load(func() ([]File, error) { return l.GetFiles(ctx, s.ID) })
}
func (s *Story) LoadPrimaryFile(ctx context.Context, l FileGetter) error {
	return s.Files.loadPrimary(func() (File, error) {
		if s.PrimaryFileID == nil {
			return nil, nil
		}
		files, err := l.Find(ctx, *s.PrimaryFileID)
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			return files[0], nil
		}
		return nil, nil
	})
}
func (s *Story) LoadRelationships(ctx context.Context, l StoryReader) error {
	if err := s.LoadURLs(ctx, l); err != nil {
		return err
	}
	if err := s.LoadTagIDs(ctx, l); err != nil {
		return err
	}
	return s.LoadFiles(ctx, l)
}
