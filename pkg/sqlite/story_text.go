package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

// UpdateText stores the story's text. An empty text removes it. Zero bytes are taken
// out and bytes that aren't UTF-8 become "�", whichever way the text arrived,
// such as a hand-edited import file, since PostgreSQL can store neither in text.
func (qb *StoryStore) UpdateText(ctx context.Context, storyID int, text string) error {
	text = strings.ToValidUTF8(strings.ReplaceAll(text, "\x00", ""), "\uFFFD")
	if strings.TrimSpace(text) == "" {
		text = ""
	}
	if text == "" {
		stmt := dialect.Delete(storiesTextJoinTable).Where(storiesTextJoinTable.Col(storyIDColumn).Eq(storyID))
		if _, err := exec(ctx, stmt); err != nil {
			return fmt.Errorf("removing text of story %d: %w", storyID, err)
		}
		return nil
	}

	// the text is sent apart from the query, and an error leaves out the query and its
	// values, which hold the whole story
	stmt := dialect.Insert(storiesTextJoinTable).Prepared(true).Rows(goqu.Record{
		storyIDColumn: storyID,
		"text":        text,
	}).OnConflict(goqu.DoUpdate(storyIDColumn, goqu.Record{"text": goqu.I("excluded.text")}))
	if _, err := exec(ctx, stmt); err != nil {
		return fmt.Errorf("storing text of story %d: %w", storyID, databaseError(err))
	}
	return nil
}

// databaseError is the database's own error inside err, without the query that exec
// adds around it.
func databaseError(err error) error {
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			return err
		}
		err = inner
	}
}
