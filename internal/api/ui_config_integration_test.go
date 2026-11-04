//go:build integration

package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2079: numbers inside lists sent by the UI are saved as numbers both to the
// config file and to the administrator's own UI settings in the database.
// Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestConfigureUINumbersInListsOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	configFile := filepath.Join(t.TempDir(), "config.yml")
	config.GetInstance().SetConfigFile(configFile)

	user := models.User{Username: "settings-admin", Roles: models.Roles{models.RoleEnumAdmin}}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		return db.User().Create(ctx, &user, "")
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}}
	gql := newTestGraphQLServer(r)

	body := customFieldsRequest(t, gql, user, `mutation($ui: Map) { configureUI(input: $ui) }`,
		`{"ui":{"frontPageContent":[{"__typename":"SavedFilter","savedFilterId":3},{"nested":[[4.5]]}],"plain":7}}`)
	require.JSONEq(t, `{"data":{"configureUI":{"frontPageContent":[{"__typename":"SavedFilter","savedFilterId":3},{"nested":[[4.5]]}],"plain":7}}}`, body)

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "savedFilterId: 3\n")
	assert.Contains(t, string(data), "- - 4.5\n")

	var stored map[string]interface{}
	require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
		var err error
		stored, err = db.UserUIConfig().Get(models.WithUserID(ctx, user.ID))
		return err
	}))
	// the database stores the settings as JSON, which reads back as float64
	assert.Equal(t, map[string]interface{}{
		"frontPageContent": []interface{}{
			map[string]interface{}{"__typename": "SavedFilter", "savedFilterId": 3.0},
			map[string]interface{}{"nested": []interface{}{[]interface{}{4.5}}},
		},
		"plain": 7.0,
	}, stored)

	body = customFieldsRequest(t, gql, user, `mutation($ui: Map) { configureUI(input: $ui) }`,
		`{"ui":{"list":[1e400]}}`)
	assert.Contains(t, body, `\"list\"[0]: invalid number \"1e400\"`)
}
