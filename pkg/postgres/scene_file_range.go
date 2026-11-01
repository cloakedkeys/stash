package postgres

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"gopkg.in/guregu/null.v4"

	"github.com/stashapp/stash/pkg/models"
)

// sceneFilesTable links scenes to their files. The link to a scene's primary file
// also holds where the scene starts and ends in it.
type sceneFilesTable struct {
	relatedFilesTable
}

type sceneFileRange struct {
	FileID    models.FileID `db:"file_id"`
	StartTime null.Float    `db:"start_time"`
	EndTime   null.Float    `db:"end_time"`
}

func (t *sceneFilesTable) setPrimaryRange(ctx context.Context, id int, start, end models.OptionalFloat64) error {
	r := updateRecord{Record: make(exp.Record)}
	r.setNullFloat64("start_time", start)
	r.setNullFloat64("end_time", end)

	table := t.table.table
	q := dialect.Update(table).Prepared(true).Set(r.Record).Where(t.idColumn.Eq(id), table.Col("primary").IsTrue())
	if _, err := exec(ctx, q); err != nil {
		return fmt.Errorf("setting range in %s: %w", table.GetTable(), err)
	}

	return nil
}

func (t *sceneFilesTable) primaryRange(ctx context.Context, id int) (*sceneFileRange, error) {
	table := t.table.table
	sql, args, err := dialect.Select("file_id", "start_time", "end_time").From(table).Prepared(true).Where(t.idColumn.Eq(id), table.Col("primary").IsTrue()).ToSQL()
	if err != nil {
		return nil, err
	}

	var ret []sceneFileRange
	if err := querySelect(ctx, sql, args, &ret); err != nil {
		return nil, fmt.Errorf("getting range from %s: %w", table.GetTable(), err)
	}
	if len(ret) == 0 {
		return nil, nil
	}

	return &ret[0], nil
}

// replaceJoins keeps the scene's range when its primary file stays the same.
func (t *sceneFilesTable) replaceJoins(ctx context.Context, id int, fileIDs []models.FileID) error {
	old, err := t.primaryRange(ctx, id)
	if err != nil {
		return err
	}

	if err := t.relatedFilesTable.replaceJoins(ctx, id, fileIDs); err != nil {
		return err
	}

	if old == nil || len(fileIDs) == 0 || fileIDs[0] != old.FileID || (!old.StartTime.Valid && !old.EndTime.Valid) {
		return nil
	}

	table := t.table.table
	q := dialect.Update(table).Prepared(true).Set(goqu.Record{
		"start_time": old.StartTime,
		"end_time":   old.EndTime,
	}).Where(t.idColumn.Eq(id), table.Col(fileIDColumn).Eq(old.FileID))
	if _, err := exec(ctx, q); err != nil {
		return fmt.Errorf("keeping range in %s: %w", table.GetTable(), err)
	}

	return nil
}
