//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDF6Migration(t *testing.T) {
	config.InitializeEmpty()
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = migrationFixture(t)
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < 31; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	for _, q := range []string{
		"INSERT INTO performers (id,name,created_at,updated_at) VALUES (901,'Existing performer',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
		"INSERT INTO tags (id,name,created_at,updated_at) VALUES (902,'Existing tag',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
		"INSERT INTO scenes (id,title,created_at,updated_at) VALUES (903,'Existing scene',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
		"INSERT INTO scenes_user_data (scene_id,user_id,rating,resume_time,play_duration) VALUES (903,1,73,12.5,30)",
	} {
		_, err = m.conn.Exec(q)
		require.NoError(t, err)
	}
	require.NoError(t, m.RunMigration(ctx, 31))
	var performer struct {
		Name      string
		Organized bool
		SortName  sql.NullString `db:"sort_name"`
	}
	require.NoError(t, m.conn.Get(&performer, "SELECT name,organized,sort_name FROM performers WHERE id=901"))
	assert.Equal(t, "Existing performer", performer.Name)
	assert.False(t, performer.Organized)
	assert.False(t, performer.SortName.Valid)
	var tag struct {
		Name      string
		Organized bool
	}
	require.NoError(t, m.conn.Get(&tag, "SELECT name,organized FROM tags WHERE id=902"))
	assert.Equal(t, "Existing tag", tag.Name)
	assert.False(t, tag.Organized)
	var data struct {
		Rating       int
		ResumeTime   float64       `db:"resume_time"`
		PlayDuration float64       `db:"play_duration"`
		WatchLaterAt NullTimestamp `db:"watch_later_at"`
	}
	require.NoError(t, m.conn.Get(&data, "SELECT rating,resume_time,play_duration,watch_later_at FROM scenes_user_data WHERE scene_id=903 AND user_id=1"))
	assert.Equal(t, 73, data.Rating)
	assert.Equal(t, 12.5, data.ResumeTime)
	assert.Equal(t, float64(30), data.PlayDuration)
	assert.False(t, data.WatchLaterAt.Valid)

	migration, err := m.getMigrate()
	require.NoError(t, err)
	require.NoError(t, migration.Steps(-1))
	for table, columns := range map[string][]string{
		"performers":       {"organized", "sort_name"},
		"tags":             {"organized"},
		"scenes_user_data": {"watch_later_at"},
	} {
		for _, column := range columns {
			var count int
			require.NoError(t, m.conn.Get(&count, "SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2", table, column))
			assert.Zero(t, count, table+"."+column)
		}
	}
	for table, want := range map[string]string{"performers": "Existing performer", "tags": "Existing tag"} {
		var name string
		require.NoError(t, m.conn.Get(&name, "SELECT name FROM "+table))
		assert.Equal(t, want, name)
	}
	require.NoError(t, m.conn.Get(&data.Rating, "SELECT rating FROM scenes_user_data WHERE scene_id=903 AND user_id=1"))
	assert.Equal(t, 73, data.Rating)
}
