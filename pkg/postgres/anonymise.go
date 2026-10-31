package postgres

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

const (
	batchSize = 5000
)

type Anonymiser struct {
	*sqlite.Database
	sourceDB *Database
}

func NewAnonymiser(db *Database, outPath string) (*sqlite.Anonymiser, error) {
	newDB := &Anonymiser{Database: sqlite.NewDatabase(), sourceDB: db}
	if err := newDB.Open(context.Background(), outPath); err != nil {
		return nil, fmt.Errorf("opening %s: %w", outPath, err)
	}

	return sqlite.PassAnonymiser(newDB)
}

func (db *Anonymiser) GetSqliteDatabase() *sqlite.Database {
	return db.Database
}

func (db *Anonymiser) FetchAll(ctx context.Context) error {
	var sqlite_dialect = goqu.Dialect("sqlite3")

	ctx, err := db.Begin(ctx, true)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	for _, table := range []exp.IdentifierExpression{
		goqu.I(audioFileTable),
		goqu.I(audioTable),
		goqu.I(audiosCustomFieldsTable.GetTable()),
		goqu.I(audiosFilesTable),
		goqu.I(audiosGalleriesTable),
		goqu.I(audiosODatesTable),
		goqu.I(audiosPlayDatesTable),
		goqu.I(audiosTagsTable),
		goqu.I(audiosURLsTable),
		goqu.I(audiosUserDataTable),
		goqu.I(galleriesCustomFieldsTable.GetTable()),
		goqu.I(galleriesUserDataTable),
		goqu.I(groupAliasesTable),
		goqu.I(groupsAudiosTable),
		goqu.I(groupsCustomFieldsTable.GetTable()),
		goqu.I(groupsUserDataTable),
		goqu.I(imagesCustomFieldsTable.GetTable()),
		goqu.I(imagesUserDataTable),
		goqu.I(performersAudiosTable),
		goqu.I(performersCustomFieldsTable.GetTable()),
		goqu.I(performersUserDataTable),
		goqu.I(scenesCustomFieldsTable.GetTable()),
		goqu.I("scene_stash_ids"),
		goqu.I(scenesUserDataTable),
		goqu.I("scenes_voice_actors"),
		goqu.I(studiosCustomFieldsTable.GetTable()),
		goqu.I(studioURLsTable),
		goqu.I(studiosUserDataTable),
		goqu.I(tagsCustomFieldsTable.GetTable()),
		goqu.I(tagsUserDataTable),
		goqu.I(userRolesTable),
		goqu.I(userTable),
		goqu.I(userUIConfigTable),
		goqu.I("voice_actors"),
		goqu.I("voice_actors_user_data"),
		goqu.I(fileTable),
		goqu.I(fingerprintTable),
		goqu.I(folderTable),
		goqu.I(galleryTable),
		goqu.I(galleriesChaptersTable),
		goqu.I(galleriesFilesTable),
		goqu.I(galleriesImagesTable),
		goqu.I(galleriesTagsTable),
		goqu.I(galleriesURLsTable),
		goqu.I(groupURLsTable),
		goqu.I(groupTable),
		goqu.I(groupRelationsTable),
		goqu.I(groupsScenesTable),
		goqu.I(groupsTagsTable),
		goqu.I(imageFileTable),
		goqu.I(imagesURLsTable),
		goqu.I(imageTable),
		goqu.I(imagesFilesTable),
		goqu.I(imagesTagsTable),
		goqu.I(performersAliasesTable),
		goqu.I("performer_stash_ids"),
		goqu.I(performerURLsTable),
		goqu.I(performerTable),
		goqu.I(performersGalleriesTable),
		goqu.I(performersImagesTable),
		goqu.I(performersScenesTable),
		goqu.I(performersTagsTable),
		goqu.I(savedFilterTable),
		goqu.I(sceneMarkerTable),
		goqu.I("scene_markers_tags"),
		goqu.I(scenesURLsTable),
		goqu.I(sceneTable),
		goqu.I(scenesFilesTable),
		goqu.I(scenesGalleriesTable),
		goqu.I(scenesODatesTable),
		goqu.I(scenesTagsTable),
		goqu.I(scenesViewDatesTable),
		goqu.I(studioAliasesTable),
		goqu.I("studio_stash_ids"),
		goqu.I(studioTable),
		goqu.I(studiosTagsTable),
		goqu.I(tagAliasesTable),
		goqu.I(tagTable),
		goqu.I(tagRelationsTable),
		goqu.I("tag_stash_ids"),
		goqu.I(videoCaptionsTable),
		goqu.I(videoFileTable),
	} {
		// the migrations of the new database add some rows of their own
		if _, _, err := db.ExecSQL(ctx, fmt.Sprintf("DELETE FROM `%s`", table.GetCol()), nil); err != nil {
			return fmt.Errorf("clearing %s: %w", table.GetCol(), err)
		}

		offset := 0
		for {
			q := dialect.From(table).Select(table.All()).Limit(uint(batchSize)).Offset(uint(offset))
			var rowsSlice []map[string]interface{}

			// Fetch
			if err := txn.WithTxn(ctx, db.sourceDB, func(ctx context.Context) error {
				// queryFunc calls this once for each row
				if err := queryFunc(ctx, q, false, func(r *sqlx.Rows) error {
					row := make(map[string]interface{})
					if err := r.MapScan(row); err != nil {
						return fmt.Errorf("failed structscan: %w", err)
					}
					rowsSlice = append(rowsSlice, row)

					return nil
				}); err != nil {
					return fmt.Errorf("querying %s: %w", table, err)
				}

				return nil
			}); err != nil {
				return fmt.Errorf("failed fetch transaction: %w", err)
			}

			if len(rowsSlice) == 0 {
				break
			}

			// the two schemas are maintained separately, so columns that only exist
			// here have nowhere to go
			columns, err := db.sqliteColumns(ctx, fmt.Sprint(table.GetCol()))
			if err != nil {
				return err
			}
			for _, row := range rowsSlice {
				for column := range row {
					if !columns[column] {
						delete(row, column)
					}
				}
			}

			// Insert
			i := sqlite_dialect.Insert(table).Rows(rowsSlice)
			sql, args, err := i.ToSQL()
			if err != nil {
				return fmt.Errorf("failed tosql: %w", err)
			}

			_, _, err = db.ExecSQL(ctx, sql, args)
			if err != nil {
				return fmt.Errorf("exec `%s` [%v]: %w", sql, args, err)
			}

			// Move to the next batch
			offset += batchSize
		}
	}

	if err := db.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (db *Anonymiser) sqliteColumns(ctx context.Context, table string) (map[string]bool, error) {
	_, rows, err := db.QuerySQL(ctx, "SELECT name FROM pragma_table_info(?)", []interface{}{table})
	if err != nil {
		return nil, fmt.Errorf("getting columns of %s: %w", table, err)
	}

	ret := make(map[string]bool, len(rows))
	for _, row := range rows {
		name, ok := row[0].(string)
		if !ok {
			return nil, fmt.Errorf("unexpected column name %v in %s", row[0], table)
		}
		ret[name] = true
	}

	return ret, nil
}
