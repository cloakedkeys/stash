package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
)

// The text of a story that has no text file, such as one scraped from a website.

const storiesTextTable = "stories_text"

var storiesTextJoinTable = goqu.T(storiesTextTable)

// GetText returns the story's stored text, or nil if it has none.
func (qb *StoryStore) GetText(ctx context.Context, storyID int) (*string, error) {
	q := dialect.From(storiesTextJoinTable).Select(storiesTextJoinTable.Col("text")).Where(storiesTextJoinTable.Col(storyIDColumn).Eq(storyID))
	var text sql.NullString
	if err := querySimple(ctx, q, &text); err != nil {
		return nil, fmt.Errorf("getting text of story %d: %w", storyID, err)
	}
	if !text.Valid {
		return nil, nil
	}
	return &text.String, nil
}

// HasText reports whether the story has stored text, without loading it.
func (qb *StoryStore) HasText(ctx context.Context, storyID int) (bool, error) {
	q := dialect.From(storiesTextJoinTable).Select(goqu.L("1")).Where(storiesTextJoinTable.Col(storyIDColumn).Eq(storyID))
	var found sql.NullInt64
	if err := querySimple(ctx, q, &found); err != nil {
		return false, fmt.Errorf("checking text of story %d: %w", storyID, err)
	}
	return found.Valid, nil
}

// UpdateText stores the story's text. An empty text removes it.
func (qb *StoryStore) UpdateText(ctx context.Context, storyID int, text string) error {
	if text == "" {
		stmt := dialect.Delete(storiesTextJoinTable).Where(storiesTextJoinTable.Col(storyIDColumn).Eq(storyID))
		if _, err := exec(ctx, stmt); err != nil {
			return fmt.Errorf("removing text of story %d: %w", storyID, err)
		}
		return nil
	}

	stmt := dialect.Insert(storiesTextJoinTable).Rows(goqu.Record{
		storyIDColumn: storyID,
		"text":        text,
	}).OnConflict(goqu.DoUpdate(storyIDColumn, goqu.Record{"text": goqu.I("excluded.text")}))
	if _, err := exec(ctx, stmt); err != nil {
		return fmt.Errorf("storing text of story %d: %w", storyID, err)
	}
	return nil
}
