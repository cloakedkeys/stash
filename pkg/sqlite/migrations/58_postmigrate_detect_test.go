package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMigration58IsSnakeCaseConfig(t *testing.T) {
	m := schema58Migrator{}
	for _, c := range []struct {
		name string
		ui   map[string]interface{}
		want bool
	}{
		{"old", map[string]interface{}{"show_studio_as_text": true, "front_page_content": []interface{}{map[string]interface{}{"savedfilterid": 1}}}, true},
		{"current", map[string]interface{}{"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{"object_filter": map[string]interface{}{}}}}, false},
		{"converted", map[string]interface{}{"showStudioAsText": true}, false},
		// nothing to convert
		{"one plain key", map[string]interface{}{"lang": "en"}, false},
		{"leading underscore only", map[string]interface{}{"__typename": "x"}, false},
		// an uppercase letter in a list is enough
		{"uppercase in a list", map[string]interface{}{"front_page_content": []interface{}{map[string]interface{}{"savedFilterId": 1}}}, false},
		// as the YAML parser of the time could return it
		{"yaml map", map[string]interface{}{"task_defaults": map[interface{}]interface{}{"scan": true}}, true},
	} {
		assert.Equal(t, c.want, m.isSnakeCaseConfig(c.ui), c.name)
	}
}
