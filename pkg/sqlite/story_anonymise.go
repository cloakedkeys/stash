package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/txn"
)

func (db *Anonymiser) anonymiseStories(ctx context.Context) error {
	logger.Infof("Anonymising stories")
	table := storyTableMgr.table
	lastID := 0
	total := 0
	const logEvery = 10000

	for gotSome := true; gotSome; {
		if err := txn.WithTxn(ctx, db, func(ctx context.Context) error {
			query := dialect.From(table).Select(
				table.Col(idColumn),
				table.Col("title"),
				table.Col("details"),
				table.Col("author"),
				table.Col("source_website"),
				table.Col("language"),
				table.Col("tagline"),
			).Where(table.Col(idColumn).Gt(lastID)).Limit(1000)

			gotSome = false

			const single = false
			return queryFunc(ctx, query, single, func(rows *sqlx.Rows) error {
				var (
					id                                       int
					title                                    sql.NullString
					details                                  sql.NullString
					author, sourceWebsite, language, tagline sql.NullString
				)

				if err := rows.Scan(
					&id,
					&title,
					&details,
					&author, &sourceWebsite, &language, &tagline,
				); err != nil {
					return err
				}

				set := goqu.Record{}
				db.obfuscateNullString(set, "title", title)
				db.obfuscateNullString(set, "details", details)
				db.obfuscateNullString(set, "author", author)
				db.obfuscateNullString(set, "source_website", sourceWebsite)
				db.obfuscateNullString(set, "language", language)
				db.obfuscateNullString(set, "tagline", tagline)

				if len(set) > 0 {
					stmt := dialect.Update(table).Set(set).Where(table.Col(idColumn).Eq(id))

					if _, err := exec(ctx, stmt); err != nil {
						return fmt.Errorf("anonymising %s: %w", table.GetTable(), err)
					}
				}

				lastID = id
				gotSome = true
				total++

				if total%logEvery == 0 {
					logger.Infof("Anonymised %d stories", total)
				}

				return nil
			})
		}); err != nil {
			return err
		}
	}

	if err := db.anonymiseURLs(ctx, goqu.T(storiesURLsTable), "story_id"); err != nil {
		return err
	}

	return nil
}
