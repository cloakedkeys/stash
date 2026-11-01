//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An existing scene keeps its file, and plays the whole file, after the upgrade.
func TestSceneFileRangesMigration(t *testing.T) {
	_ = config.InitializeEmpty()
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "ranges.sqlite")
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < 104; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	for _, q := range []string{
		"INSERT INTO folders (id,path,basename,mod_time,created_at,updated_at) VALUES (901,'/videos','videos',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
		"INSERT INTO files (id,basename,parent_folder_id,size,mod_time,created_at,updated_at) VALUES (902,'a.mp4',901,10,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
		"INSERT INTO scenes (id,title,created_at,updated_at) VALUES (903,'Existing scene',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
		"INSERT INTO scenes_files (scene_id,file_id,`primary`) VALUES (903,902,1)",
	} {
		_, err = m.conn.Exec(q)
		require.NoError(t, err, q)
	}

	require.NoError(t, m.RunMigration(ctx, 104))

	var row struct {
		FileID    int             `db:"file_id"`
		Primary   bool            `db:"primary"`
		StartTime sql.NullFloat64 `db:"start_time"`
		EndTime   sql.NullFloat64 `db:"end_time"`
	}
	require.NoError(t, m.conn.Get(&row, "SELECT file_id, `primary`, start_time, end_time FROM scenes_files WHERE scene_id=903"))
	assert.Equal(t, 902, row.FileID)
	assert.True(t, row.Primary)
	assert.False(t, row.StartTime.Valid)
	assert.False(t, row.EndTime.Valid)
}
