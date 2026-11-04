//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wildcardCase is a saved scene filter that used _ or % as wildcards, and the
// WHERE clause the old code built for it.
type wildcardCase struct {
	name   string
	filter string
	// oldWhere is on scenes; {fp} is the full path of one of the scene's files
	oldWhere string
	// scenes the rewritten filter also finds that the old one didn't
	newAlsoFinds []int
}

var wildcardCases = []wildcardCase{
	{"title includes", `{"title":{"value":"part_1","modifier":"INCLUDES"}}`,
		`scenes.title ILIKE '%part_1%' ESCAPE ''`, nil},
	{"title excludes", `{"title":{"value":"part_1","modifier":"EXCLUDES"}}`,
		`(scenes.title IS NULL OR (scenes.title NOT ILIKE '%part_1%' ESCAPE ''))`, nil},
	{"title includes words", `{"title":{"value":"foo part_1","modifier":"INCLUDES"}}`,
		`(scenes.title ILIKE '%foo%' ESCAPE '' OR scenes.title ILIKE '%part_1%' ESCAPE '')`, nil},
	{"title includes quoted phrase", `{"title":{"value":"\"100% real\"","modifier":"INCLUDES"}}`,
		`scenes.title ILIKE '%100% real%' ESCAPE ''`, nil},
	{"title equals", `{"title":{"value":"part_1","modifier":"EQUALS"}}`,
		`scenes.title ILIKE 'part_1' ESCAPE ''`, nil},
	{"title not equals", `{"title":{"value":"part_1","modifier":"NOT_EQUALS"}}`,
		`(scenes.title IS NULL OR scenes.title NOT ILIKE 'part_1' ESCAPE '')`, nil},
	// \ was a plain character: ESCAPE ''
	{"backslash", `{"title":{"value":"back\\sl_sh","modifier":"EQUALS"}}`,
		`scenes.title ILIKE 'back\sl_sh' ESCAPE ''`, nil},
	// ILIKE ignored case for every letter: été_a found ÉTÉ-A
	{"letters outside A to Z", `{"title":{"value":"été_a","modifier":"INCLUDES"}}`,
		`scenes.title ILIKE '%été_a%' ESCAPE ''`, nil},
	{"details across lines", `{"details":{"value":"line_one%end","modifier":"INCLUDES"}}`,
		`scenes.details ILIKE '%line_one%end%' ESCAPE ''`, nil},
	{"sub-filter", `{"AND":{"title":{"value":"p%1","modifier":"EQUALS"}}}`,
		`scenes.title ILIKE 'p%1' ESCAPE ''`, nil},
	{"path includes", `{"path":{"value":"part_1","modifier":"INCLUDES"}}`,
		`EXISTS (SELECT 1 FROM scenes_files sf JOIN files f ON f.id = sf.file_id JOIN folders fo ON fo.id = f.parent_folder_id WHERE sf.scene_id = scenes.id AND {fp} ILIKE '%part_1%' ESCAPE '')`, nil},
	{"path equals", `{"path":{"value":"%part_1.mp4","modifier":"EQUALS"}}`,
		`EXISTS (SELECT 1 FROM scenes_files sf JOIN files f ON f.id = sf.file_id JOIN folders fo ON fo.id = f.parent_folder_id WHERE sf.scene_id = scenes.id AND {fp} ILIKE '%part_1.mp4' ESCAPE '')`, nil},
	// the old excludes joined the scene's files, so a scene without files was
	// never found; "doesn't match regex" keeps it, as it does for an empty title
	{"path excludes", `{"path":{"value":"part_1","modifier":"EXCLUDES"}}`,
		`EXISTS (SELECT 1 FROM scenes_files sf JOIN files f ON f.id = sf.file_id JOIN folders fo ON fo.id = f.parent_folder_id WHERE sf.scene_id = scenes.id AND NOT ({fp} ILIKE '%part_1%' ESCAPE ''))`, []int{10}},

	// custom fields: includes needed every value, each as typed; excludes
	// found scenes without the field too. A scene has one row per field.
	{"custom field includes", `{"custom_fields":[{"field":"source","value":["site_a"],"modifier":"INCLUDES"}]}`,
		`EXISTS (SELECT 1 FROM scene_custom_fields c WHERE c.scene_id = scenes.id AND c.field = 'source' AND c.value->>0 ILIKE '%site_a%' ESCAPE '')`, nil},
	{"custom field includes every value", `{"custom_fields":[{"field":"source","value":["site_","%a"],"modifier":"INCLUDES"}]}`,
		`EXISTS (SELECT 1 FROM scene_custom_fields c WHERE c.scene_id = scenes.id AND c.field = 'source' AND c.value->>0 ILIKE '%site_%' ESCAPE '' AND c.value->>0 ILIKE '%%a%' ESCAPE '')`, nil},
	{"custom field includes percent", `{"custom_fields":[{"field":"source","value":["100%"],"modifier":"INCLUDES"}]}`,
		`EXISTS (SELECT 1 FROM scene_custom_fields c WHERE c.scene_id = scenes.id AND c.field = 'source' AND c.value->>0 ILIKE '%100%%' ESCAPE '')`, nil},
	{"custom field includes a number", `{"custom_fields":[{"field":"source","value":["%5"],"modifier":"INCLUDES"}]}`,
		`EXISTS (SELECT 1 FROM scene_custom_fields c WHERE c.scene_id = scenes.id AND c.field = 'source' AND c.value->>0 ILIKE '%%5%' ESCAPE '')`, nil},
	// ILIKE ignored case for every letter
	{"custom field letters outside A to Z", `{"custom_fields":[{"field":"source","value":["été_a"],"modifier":"INCLUDES"}]}`,
		`EXISTS (SELECT 1 FROM scene_custom_fields c WHERE c.scene_id = scenes.id AND c.field = 'source' AND c.value->>0 ILIKE '%été_a%' ESCAPE '')`, nil},
	// excludes had no ESCAPE clause
	{"custom field excludes", `{"custom_fields":[{"field":"source","value":["site_a"],"modifier":"EXCLUDES"}]}`,
		`NOT EXISTS (SELECT 1 FROM scene_custom_fields c WHERE c.scene_id = scenes.id AND c.field = 'source' AND c.value->>0 ILIKE '%site_a%')`, nil},
	// so \ escaped the next character: a\b meant ab
	{"custom field excludes backslash", `{"custom_fields":[{"field":"source","value":["a\\b"],"modifier":"EXCLUDES"}]}`,
		`NOT EXISTS (SELECT 1 FROM scene_custom_fields c WHERE c.scene_id = scenes.id AND c.field = 'source' AND c.value->>0 ILIKE '%a\b%')`, nil},
}

// filters left as they were
var untouchedFilters = []string{
	`{"title":{"value":"part-1","modifier":"INCLUDES"}}`,
	`{"resolution":{"value":"FULL_HD","modifier":"EQUALS"}}`,
	`{"url":{"value":"site_x","modifier":"EXCLUDES"}}`,
	// equals didn't use LIKE
	`{"custom_fields":[{"field":"source","value":["site_a"],"modifier":"EQUALS"}]}`,
	// \ was a plain character in custom field includes (ESCAPE '')
	`{"custom_fields":[{"field":"source","value":["a\\b"],"modifier":"INCLUDES"}]}`,
}

func migration26SceneIDs(t *testing.T, ctx context.Context, db *Database, objectFilter map[string]interface{}) []int {
	t.Helper()
	data, err := json.Marshal(objectFilter)
	require.NoError(t, err)
	var f models.SceneFilterType
	require.NoError(t, json.Unmarshal(data, &f))
	all := -1
	var ids []int
	require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
		r, err := db.Scene().Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: &models.FindFilterType{PerPage: &all}},
			SceneFilter:  &f,
		})
		if err != nil {
			return err
		}
		ids = r.IDs
		return nil
	}))
	ids = append([]int{}, ids...)
	sort.Ints(ids)
	return ids
}

// The config file's default filters are rewritten once per config file, not
// each time a new database is created: a new database runs migration 26 too,
// and so does the PostgreSQL database a SQLite one is copied to.
func TestMigration26ConfigFileOnce(t *testing.T) {
	// the subtests open databases with the parent t, so skip here rather than in them
	if os.Getenv("PGSQL_TEST") == "" {
		t.Skip("PGSQL_TEST is required")
	}
	ctx := context.Background()
	config.InitializeEmpty()
	cfg := config.GetInstance()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	saveDefault := func(value string) {
		t.Helper()
		cfg.SetUIConfiguration(map[string]interface{}{"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{
			"mode":          "SCENES",
			"object_filter": map[string]interface{}{"title": map[string]interface{}{"value": value, "modifier": "EQUALS"}},
		}}})
		require.NoError(t, cfg.Write())
	}
	defaultTitle := func() interface{} {
		t.Helper()
		scenes := cfg.GetUIConfiguration()["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
		return scenes["object_filter"].(map[string]interface{})["title"]
	}
	newDatabase := func() {
		t.Helper()
		db := NewDatabase()
		require.NoError(t, db.Open(ctx, migrationFixture(t)))
		require.Equal(t, appSchemaVersion, db.schemaVersion)
		require.NoError(t, db.Close())
	}
	literal := map[string]interface{}{"value": "a_b", "modifier": "EQUALS"}

	t.Run("a marked config file is left alone", func(t *testing.T) {
		cfg.SetBool(config.LegacyWildcardsConverted, true)
		saveDefault("a_b")
		newDatabase()
		assert.Equal(t, literal, defaultTitle())
	})

	t.Run("an unmarked one is rewritten once", func(t *testing.T) {
		cfg.SetInterface(config.LegacyWildcardsConverted, nil)
		saveDefault("a_b")
		newDatabase()
		assert.Equal(t, map[string]interface{}{"value": "(?is)^a.b$", "modifier": "MATCHES_REGEX"}, defaultTitle())
		assert.True(t, cfg.GetLegacyWildcardsConverted())

		// a default filter this build saves, then another new database
		saveDefault("a_b")
		newDatabase()
		assert.Equal(t, literal, defaultTitle())
	})
}

// Saved filters and default filters that used _ or % as wildcards find the
// same scenes after the upgrade as before it.
func TestMigration26SavedFilterWildcards(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()
	db := NewDatabase()
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	for version := uint(1); version <= 25; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}

	_, err = m.conn.Exec(`
INSERT INTO scenes(id,title,details,created_at,updated_at) VALUES
 (1,'part_1',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (2,'part-1',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (3,'part 1',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (4,'parts',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (5,'PART-1',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (6,'100% real',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (7,'100 percent real',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (8,NULL,'line-one
middle
end',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (9,'apart_12','line one end',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (10,'p1',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (11,'back\slash',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (12,'backslash',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (13,'ÉTÉ-A',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (14,'été-a',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO folders(id,basename,path,mod_time,created_at,updated_at) VALUES (1,'videos','/videos',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO files(id,basename,parent_folder_id,size,mod_time,created_at,updated_at) VALUES
 (1,'part_1.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (2,'part-1.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (3,'part 1.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (4,'parts.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (5,'other.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (6,'six.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (7,'seven.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (8,'eight.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (9,'apart_12.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (11,'eleven.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (12,'twelve.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (13,'thirteen.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 (14,'fourteen.mp4',1,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO scenes_files(scene_id,file_id,"primary") VALUES (1,1,true),(2,2,true),(3,3,true),(4,4,true),(5,5,true),(6,6,true),(7,7,true),(8,8,true),(9,9,true),(11,11,true),(12,12,true),(13,13,true),(14,14,true);
INSERT INTO scene_custom_fields(scene_id,field,value,type) VALUES
 (1,'source','"site_a"','string'),(2,'source','"site-a"','string'),(3,'source','"site a"','string'),(4,'source','"SITE-A"','string'),
 (5,'source','"other"','string'),(6,'source','5','int64'),(7,'source','"100% real"','string'),(8,'source','"100 real"','string'),
 (9,'source','"a\\b"','string'),(11,'source','"ab"','string'),(13,'source','"ÉTÉ-A"','string'),(14,'source','"été-a"','string');`)
	require.NoError(t, err)

	fp := "fo.path || '" + string(filepath.Separator) + "' || f.basename"
	expected := make([][]int, len(wildcardCases))
	for i, c := range wildcardCases {
		var ids []int
		require.NoError(t, m.conn.Select(&ids, "SELECT id FROM scenes WHERE "+strings.ReplaceAll(c.oldWhere, "{fp}", fp)))
		ids = append(ids, c.newAlsoFinds...)
		sort.Ints(ids)
		expected[i] = ids
		_, err := m.conn.Exec(`INSERT INTO saved_filters(id,user_id,name,mode,find_filter,object_filter,ui_options) VALUES($1,1,$2,'SCENES','{"q":""}'::jsonb,$3::jsonb,'{}'::jsonb)`, i+1, c.name, c.filter)
		require.NoError(t, err)
	}
	for i, f := range untouchedFilters {
		_, err := m.conn.Exec(`INSERT INTO saved_filters(id,user_id,name,mode,object_filter) VALUES($1,1,$2,'SCENES',$3::jsonb)`, 100+i, "untouched "+f, f)
		require.NoError(t, err)
	}
	uiConfig := `{"theme":"kept","defaultFilters":{"scenes":{"mode":"SCENES","find_filter":{"q":"a_b"},"object_filter":` + wildcardCases[0].filter + `}}}`
	_, err = m.conn.Exec(`INSERT INTO user_ui_config(user_id,config) VALUES(1,$1)`, uiConfig)
	require.NoError(t, err)
	var untouchedBefore []string
	require.NoError(t, m.conn.Select(&untouchedBefore, `SELECT object_filter::text FROM saved_filters WHERE id >= 100 ORDER BY id`))

	// the config file's default filters, which users without their own use
	cfg := config.GetInstance()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	var fileFilter map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(wildcardCases[0].filter), &fileFilter))
	cfg.SetUIConfiguration(map[string]interface{}{
		"theme":          "file",
		"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{"mode": "SCENES", "object_filter": fileFilter}},
	})
	require.NoError(t, cfg.Write())

	require.NoError(t, m.RunMigration(ctx, 26))
	m.Close()
	require.NoError(t, db.ReInitialise())

	userCtx := models.WithUserID(ctx, 1)
	for i, c := range wildcardCases {
		t.Run(c.name, func(t *testing.T) {
			var saved *models.SavedFilter
			require.NoError(t, txn.WithReadTxn(userCtx, db, func(ctx context.Context) error {
				var err error
				saved, err = db.SavedFilter().Find(ctx, i+1)
				return err
			}))
			require.NotNil(t, saved)
			t.Logf("the old query finds %v; saved filter is now %v", expected[i], saved.ObjectFilter)

			var original map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(c.filter), &original))
			// the filter as it was saved finds something else now, which is why it's rewritten
			require.NotEqual(t, expected[i], migration26SceneIDs(t, userCtx, db, original), "test case doesn't show a difference")
			assert.Equal(t, expected[i], migration26SceneIDs(t, userCtx, db, saved.ObjectFilter))
		})
	}

	var untouchedAfter []string
	require.NoError(t, db.readDB.Select(&untouchedAfter, `SELECT object_filter::text FROM saved_filters WHERE id >= 100 ORDER BY id`))
	assert.Equal(t, untouchedBefore, untouchedAfter)

	var ui map[string]interface{}
	require.NoError(t, txn.WithReadTxn(userCtx, db, func(ctx context.Context) error {
		var err error
		ui, err = db.UserUIConfig().Get(ctx)
		return err
	}))
	assert.Equal(t, "kept", ui["theme"])
	scenes := ui["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"q": "a_b"}, scenes["find_filter"])
	assert.Equal(t, expected[0], migration26SceneIDs(t, userCtx, db, scenes["object_filter"].(map[string]interface{})))

	fileUI := cfg.GetUIConfiguration()
	assert.Equal(t, "file", fileUI["theme"])
	fileScenes := fileUI["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
	assert.Equal(t, expected[0], migration26SceneIDs(t, userCtx, db, fileScenes["object_filter"].(map[string]interface{})))
	saved, err := os.ReadFile(cfg.GetConfigFile())
	require.NoError(t, err)
	assert.Contains(t, string(saved), "MATCHES_REGEX")
	backups, err := filepath.Glob(cfg.GetConfigFile() + ".pg25.*")
	require.NoError(t, err)
	assert.Len(t, backups, 1)
}
