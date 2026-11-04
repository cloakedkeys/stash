//go:build integration

package migrations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
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
		`scenes.title LIKE '%part_1%'`, nil},
	{"title excludes", `{"title":{"value":"part_1","modifier":"EXCLUDES"}}`,
		`(scenes.title IS NULL OR (scenes.title NOT LIKE '%part_1%'))`, nil},
	{"title includes words", `{"title":{"value":"foo part_1","modifier":"INCLUDES"}}`,
		`(scenes.title LIKE '%foo%' OR scenes.title LIKE '%part_1%')`, nil},
	{"title includes quoted phrase", `{"title":{"value":"\"100% real\"","modifier":"INCLUDES"}}`,
		`scenes.title LIKE '%100% real%'`, nil},
	{"title equals", `{"title":{"value":"part_1","modifier":"EQUALS"}}`,
		`scenes.title LIKE 'part_1'`, nil},
	{"title not equals", `{"title":{"value":"part_1","modifier":"NOT_EQUALS"}}`,
		`(scenes.title IS NULL OR scenes.title NOT LIKE 'part_1')`, nil},
	// \ was a plain character: SQLite's LIKE has no escape character unless given one
	{"backslash", `{"title":{"value":"back\\sl_sh","modifier":"EQUALS"}}`,
		`scenes.title LIKE 'back\sl_sh'`, nil},
	// LIKE ignored case only for A to Z: été_a didn't find ÉTÉ-A
	{"letters outside A to Z", `{"title":{"value":"été_a","modifier":"INCLUDES"}}`,
		`scenes.title LIKE '%été_a%'`, nil},
	{"details across lines", `{"details":{"value":"line_one%end","modifier":"INCLUDES"}}`,
		`scenes.details LIKE '%line_one%end%'`, nil},
	{"sub-filter", `{"AND":{"title":{"value":"p%1","modifier":"EQUALS"}}}`,
		`scenes.title LIKE 'p%1'`, nil},
	{"path includes", `{"path":{"value":"part_1","modifier":"INCLUDES"}}`,
		`EXISTS (SELECT 1 FROM scenes_files sf JOIN files f ON f.id = sf.file_id JOIN folders fo ON fo.id = f.parent_folder_id WHERE sf.scene_id = scenes.id AND {fp} LIKE '%part_1%')`, nil},
	{"path equals", `{"path":{"value":"%part_1.mp4","modifier":"EQUALS"}}`,
		`EXISTS (SELECT 1 FROM scenes_files sf JOIN files f ON f.id = sf.file_id JOIN folders fo ON fo.id = f.parent_folder_id WHERE sf.scene_id = scenes.id AND {fp} LIKE '%part_1.mp4')`, nil},
	// the old excludes joined the scene's files, so a scene without files was
	// never found; "doesn't match regex" keeps it, as it does for an empty title
	{"path excludes", `{"path":{"value":"part_1","modifier":"EXCLUDES"}}`,
		`EXISTS (SELECT 1 FROM scenes_files sf JOIN files f ON f.id = sf.file_id JOIN folders fo ON fo.id = f.parent_folder_id WHERE sf.scene_id = scenes.id AND NOT ({fp} LIKE '%part_1%'))`, []int{10}},
}

// filters left as they were, and the JSON they must keep
var untouchedFilters = []string{
	`{"title":{"value":"part-1","modifier":"INCLUDES"}}`,
	`{"resolution":{"value":"FULL_HD","modifier":"EQUALS"}}`,
	`{"url":{"value":"site_x","modifier":"EXCLUDES"}}`,
}

func sortedIDs(ids []int) []int {
	ret := append([]int{}, ids...)
	sort.Ints(ret)
	return ret
}

func querySceneIDs(t *testing.T, ctx context.Context, db *sqlite.Database, objectFilter map[string]interface{}) []int {
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
	return sortedIDs(ids)
}

func decodeObject(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

// databaseAt105 makes a new database at schema 105, ready for migration 106.
func databaseAt105(t *testing.T, ctx context.Context) (*sqlx.DB, *sqlite.Database, *sqlite.Migrator) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "stash.sqlite")
	raw, err := sqlx.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	initial, err := os.ReadFile("1_initial.up.sql")
	require.NoError(t, err)
	_, err = raw.Exec(string(initial) + "\nCREATE TABLE schema_migrations(version uint64, dirty bool); INSERT INTO schema_migrations VALUES(1,false);")
	require.NoError(t, err)

	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, dbPath))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	m, err := sqlite.NewMigrator(db)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	for version := uint(2); version <= 105; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	return raw, db, m
}

// The config file's default filters are rewritten once per config file, not
// each time a database reaches schema 106.
func TestMigration106ConfigFileOnce(t *testing.T) {
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
	// the default filter is saved after the earlier migrations, since 58 renames its keys
	migrateNewDatabase := func(value string) {
		t.Helper()
		_, _, m := databaseAt105(t, ctx)
		saveDefault(value)
		require.NoError(t, m.RunMigration(ctx, 106))
	}
	literal := map[string]interface{}{"value": "a_b", "modifier": "EQUALS"}

	t.Run("a marked config file is left alone", func(t *testing.T) {
		cfg.SetBool(config.LegacyWildcardsConverted, true)
		migrateNewDatabase("a_b")
		assert.Equal(t, literal, defaultTitle())
	})

	t.Run("an unmarked one is rewritten once", func(t *testing.T) {
		cfg.SetInterface(config.LegacyWildcardsConverted, nil)
		migrateNewDatabase("a_b")
		assert.Equal(t, map[string]interface{}{"value": "(?is)^a.b$", "modifier": "MATCHES_REGEX"}, defaultTitle())
		assert.True(t, cfg.GetLegacyWildcardsConverted())

		// a default filter this build saves, then another database
		migrateNewDatabase("a_b")
		assert.Equal(t, literal, defaultTitle())
	})
}

// Saved filters and default filters that used _ or % as wildcards find the
// same scenes after the upgrade as before it.
func TestMigration106SavedFilterWildcards(t *testing.T) {
	ctx := context.Background()
	config.InitializeEmpty()
	dir := t.TempDir()
	raw, db, m := databaseAt105(t, ctx)
	var err error

	_, err = raw.Exec(`
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
INSERT INTO scenes_files(scene_id,file_id,"primary") VALUES (1,1,1),(2,2,1),(3,3,1),(4,4,1),(5,5,1),(6,6,1),(7,7,1),(8,8,1),(9,9,1),(11,11,1),(12,12,1),(13,13,1),(14,14,1);`)
	require.NoError(t, err)

	fp := "fo.path || '" + string(filepath.Separator) + "' || f.basename"
	expected := make([][]int, len(wildcardCases))
	for i, c := range wildcardCases {
		var ids []int
		require.NoError(t, raw.Select(&ids, "SELECT id FROM scenes WHERE "+strings.ReplaceAll(c.oldWhere, "{fp}", fp)))
		expected[i] = sortedIDs(append(ids, c.newAlsoFinds...))
		_, err := raw.Exec("INSERT INTO saved_filters(id,user_id,name,mode,find_filter,object_filter,ui_options) VALUES(?,1,?,'SCENES','{\"q\":\"\"}',?,'{}')", i+1, c.name, c.filter)
		require.NoError(t, err)
	}
	for i, f := range untouchedFilters {
		_, err := raw.Exec("INSERT INTO saved_filters(id,user_id,name,mode,object_filter) VALUES(?,1,?,'SCENES',?)", 100+i, "untouched "+f, f)
		require.NoError(t, err)
	}
	uiConfig := `{"theme":"kept","defaultFilters":{"scenes":{"mode":"SCENES","find_filter":{"q":"a_b"},"object_filter":` + wildcardCases[0].filter + `}}}`
	_, err = raw.Exec("INSERT INTO user_ui_config(user_id,config) VALUES(1,?)", uiConfig)
	require.NoError(t, err)

	// the config file's default filters, which users without their own use
	cfg := config.GetInstance()
	cfg.SetConfigFile(filepath.Join(dir, "config.yml"))
	var fileFilter map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(wildcardCases[0].filter), &fileFilter))
	cfg.SetUIConfiguration(map[string]interface{}{
		"theme":          "file",
		"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{"mode": "SCENES", "object_filter": fileFilter}},
	})
	require.NoError(t, cfg.Write())

	require.NoError(t, m.RunMigration(ctx, 106))
	require.Equal(t, uint(106), m.CurrentSchemaVersion())

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

			// the filter as it was saved finds something else now, which is why it's rewritten
			require.NotEqual(t, expected[i], querySceneIDs(t, userCtx, db, decodeObject(t, c.filter)), "test case doesn't show a difference")
			assert.Equal(t, expected[i], querySceneIDs(t, userCtx, db, saved.ObjectFilter))
		})
	}

	for i, f := range untouchedFilters {
		var got string
		require.NoError(t, raw.Get(&got, "SELECT object_filter FROM saved_filters WHERE id = ?", 100+i))
		assert.Equal(t, f, got)
	}

	var ui map[string]interface{}
	require.NoError(t, txn.WithReadTxn(userCtx, db, func(ctx context.Context) error {
		var err error
		ui, err = db.UserUIConfig().Get(ctx)
		return err
	}))
	assert.Equal(t, "kept", ui["theme"])
	scenes := ui["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"q": "a_b"}, scenes["find_filter"])
	assert.Equal(t, expected[0], querySceneIDs(t, userCtx, db, scenes["object_filter"].(map[string]interface{})))

	fileUI := cfg.GetUIConfiguration()
	assert.Equal(t, "file", fileUI["theme"])
	fileScenes := fileUI["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
	assert.Equal(t, expected[0], querySceneIDs(t, userCtx, db, fileScenes["object_filter"].(map[string]interface{})))
	saved, err := os.ReadFile(cfg.GetConfigFile())
	require.NoError(t, err)
	assert.Contains(t, string(saved), "MATCHES_REGEX")
	backups, err := filepath.Glob(cfg.GetConfigFile() + ".105.*")
	require.NoError(t, err)
	assert.Len(t, backups, 1)
}
