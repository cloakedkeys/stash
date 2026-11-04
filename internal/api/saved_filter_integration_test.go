//go:build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const saveFilterNumbersQuery = `mutation($id: ID, $name: String!, $of: Map, $ui: Map) {
	saveFilter(input:{id:$id,mode:SCENES,name:$name,object_filter:$of,ui_options:$ui}) { id object_filter ui_options }
}`

// #2084: saveFilter refuses a number too large to read back, such as 1e400,
// in object_filter or ui_options, naming where it is, instead of saving it and
// then loading it as null. Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestSaveFilterBigNumberOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	user := models.User{Username: "filter-saver", Roles: models.Roles{models.RoleEnumModify}}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		return db.User().Create(ctx, &user, "")
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}}
	gql := newTestGraphQLServer(r)

	list := func() string {
		return customFieldsRequest(t, gql, user, `{ findSavedFilters(mode:SCENES) { name object_filter ui_options } }`, `{}`)
	}

	// numbers that fit are saved as before
	body := customFieldsRequest(t, gql, user, saveFilterNumbersQuery,
		`{"name":"good","of":{"rating100":{"value":50,"modifier":"EQUALS"},"o_counter":{"value":1e20,"modifier":"GREATER_THAN"}},"ui":{"zoom_index":1,"scale":1.5}}`)
	require.JSONEq(t, `{"data":{"saveFilter":{"id":"1","object_filter":{"rating100":{"value":50,"modifier":"EQUALS"},"o_counter":{"value":1e20,"modifier":"GREATER_THAN"}},"ui_options":{"zoom_index":1,"scale":1.5}}}}`, body)
	good := `{"data":{"findSavedFilters":[{"name":"good","object_filter":{"rating100":{"value":50,"modifier":"EQUALS"},"o_counter":{"value":1e20,"modifier":"GREATER_THAN"}},"ui_options":{"zoom_index":1,"scale":1.5}}]}}`
	require.JSONEq(t, good, list())

	for _, tc := range []struct {
		name      string
		variables string
		want      string
	}{
		{
			name:      "object_filter",
			variables: `{"name":"big","of":{"rating100":{"value":1e400,"modifier":"EQUALS"}},"ui":{}}`,
			want:      `object_filter \"rating100\".\"value\": invalid number \"1e400\"`,
		},
		{
			name:      "object_filter list",
			variables: `{"name":"big","of":{"performers":{"value":[1,1e400],"modifier":"INCLUDES"}},"ui":{}}`,
			want:      `object_filter \"performers\".\"value\"[1]: invalid number \"1e400\"`,
		},
		{
			name:      "ui_options",
			variables: `{"name":"big","of":{},"ui":{"zoom_index":1e400}}`,
			want:      `ui_options \"zoom_index\": invalid number \"1e400\"`,
		},
		{
			name:      "update object_filter",
			variables: `{"id":"1","name":"good","of":{"rating100":{"value":-1e400}},"ui":{}}`,
			want:      `object_filter \"rating100\".\"value\": invalid number \"-1e400\"`,
		},
		{
			name:      "update ui_options",
			variables: `{"id":"1","name":"good","of":{},"ui":{"zoom_index":1e400}}`,
			want:      `ui_options \"zoom_index\": invalid number \"1e400\"`,
		},
	} {
		body := customFieldsRequest(t, gql, user, saveFilterNumbersQuery, tc.variables)
		assert.Contains(t, body, tc.want, tc.name)
		assert.Contains(t, body, `"data":null`, tc.name)
		// nothing is saved or changed
		assert.JSONEq(t, good, list(), tc.name)
	}
}

// #2084: refusing 1e400 doesn't change how other numbers are saved: they are
// stored as written, such as 1.50 and 99999999999999999999, and read back as
// before. Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestSaveFilterKeepsNumbersAsWrittenOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	user := models.User{Username: "filter-numbers", Roles: models.Roles{models.RoleEnumModify}}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		return db.User().Create(ctx, &user, "")
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}}
	gql := newTestGraphQLServer(r)

	stored := func(id string) map[string]interface{} {
		var ret map[string]interface{}
		require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
			_, rows, err := db.QuerySQL(ctx, "SELECT object_filter, ui_options FROM saved_filters WHERE id = "+id, nil)
			if err != nil {
				return err
			}
			require.Len(t, rows, 1)
			ret = map[string]interface{}{}
			for i, key := range []string{"object_filter", "ui_options"} {
				var s string
				switch v := rows[0][i].(type) {
				case string:
					s = v
				case []byte:
					s = string(v)
				default:
					s = fmt.Sprint(v)
				}
				d := json.NewDecoder(strings.NewReader(s))
				d.UseNumber()
				var m interface{}
				require.NoError(t, d.Decode(&m), s)
				ret[key] = m
			}
			return nil
		}))
		return ret
	}

	want := map[string]interface{}{
		"object_filter": map[string]interface{}{"rating100": map[string]interface{}{"value": json.Number("99999999999999999999"), "value2": json.Number("1.50"), "modifier": "BETWEEN"}, "ids": []interface{}{json.Number("1.50")}},
		"ui_options":    map[string]interface{}{"scale": json.Number("1.50"), "big": json.Number("99999999999999999999")},
	}
	readBack := `{"rating100":{"value":1e20,"value2":1.5,"modifier":"BETWEEN"},"ids":[1.5]}`
	readBackUI := `{"scale":1.5,"big":1e20}`

	variables := `{"name":"numbers","of":{"rating100":{"value":99999999999999999999,"value2":1.50,"modifier":"BETWEEN"},"ids":[1.50]},"ui":{"scale":1.50,"big":99999999999999999999}}`
	body := customFieldsRequest(t, gql, user, saveFilterNumbersQuery, variables)
	require.JSONEq(t, `{"data":{"saveFilter":{"id":"1","object_filter":`+readBack+`,"ui_options":`+readBackUI+`}}}`, body)
	assert.Equal(t, want, stored("1"))

	// updating stores them as written too
	body = customFieldsRequest(t, gql, user, saveFilterNumbersQuery, `{"id":"1",`+variables[1:])
	require.Contains(t, body, `"data":{"saveFilter":{"id":"1"`)
	assert.Equal(t, want, stored("1"))

	body = customFieldsRequest(t, gql, user, `{ findSavedFilter(id:"1") { object_filter ui_options } }`, `{}`)
	require.JSONEq(t, `{"data":{"findSavedFilter":{"object_filter":`+readBack+`,"ui_options":`+readBackUI+`}}}`, body)
}

// #2084: a saved filter already holding a number too large to read back, saved
// before saveFilter refused it, still loads, with that number as null, and
// does not stop the other saved filters loading. Runs on SQLite, or PostgreSQL
// when PGSQL_TEST is set.
func TestFindSavedFiltersStoredBigNumberOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	user := models.User{Username: "filter-loader", Roles: models.Roles{models.RoleEnumModify}}
	var brokenID int
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &user, ""); err != nil {
			return err
		}
		ctx = models.WithUserID(ctx, user.ID)
		broken := models.SavedFilter{
			Mode:         models.FilterModeScenes,
			Name:         "broken",
			ObjectFilter: map[string]interface{}{"rating100": map[string]interface{}{"value": json.Number("1e400"), "modifier": "EQUALS"}},
			UIOptions:    map[string]interface{}{"zoom_index": json.Number("1e400"), "scale": json.Number("2")},
		}
		if err := db.SavedFilter().Create(ctx, &broken); err != nil {
			return err
		}
		brokenID = broken.ID
		return db.SavedFilter().Create(ctx, &models.SavedFilter{
			Mode:         models.FilterModeScenes,
			Name:         "good",
			ObjectFilter: map[string]interface{}{"rating100": map[string]interface{}{"value": 50, "modifier": "EQUALS"}},
		})
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}}
	gql := newTestGraphQLServer(r)

	brokenJSON := `{"name":"broken","object_filter":{"rating100":{"value":null,"modifier":"EQUALS"}},"ui_options":{"zoom_index":null,"scale":2}}`
	goodJSON := `{"name":"good","object_filter":{"rating100":{"value":50,"modifier":"EQUALS"}},"ui_options":null}`

	body := customFieldsRequest(t, gql, user, `{ findSavedFilters(mode:SCENES) { name object_filter ui_options } }`, `{}`)
	require.JSONEq(t, `{"data":{"findSavedFilters":[`+brokenJSON+`,`+goodJSON+`]}}`, body)

	body = customFieldsRequest(t, gql, user, fmt.Sprintf(`{ findSavedFilter(id:"%d") { name object_filter ui_options } }`, brokenID), `{}`)
	require.JSONEq(t, `{"data":{"findSavedFilter":`+brokenJSON+`}}`, body)
}
