//go:build integration

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"
)

// customFieldsRequest sends query with the variables written as raw JSON, so
// numbers such as 1e20 reach the server as typed, as they do from the UI.
func customFieldsRequest(t *testing.T, gql http.Handler, u models.User, query string, variables string) string {
	t.Helper()
	body := fmt.Sprintf(`{"query":%q,"variables":%s}`, query, variables)
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(session.SetCurrentUser(req.Context(), u))
	w := httptest.NewRecorder()
	gql.ServeHTTP(w, req)
	return w.Body.String()
}

// #2061: a custom field of 1e20 was saved as 0. Runs on SQLite, or PostgreSQL
// when PGSQL_TEST is set.
func TestPerformerUpdateCustomFieldBigNumbersOnDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := openUserDataTestDatabase(ctx, t)

	user := models.User{Username: "editor", Roles: models.Roles{models.RoleEnumModify}}
	performer := models.NewPerformer()
	performer.Name = "Someone"
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if err := db.User().Create(ctx, &user, ""); err != nil {
			return err
		}
		return db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &performer})
	}))

	r := &Resolver{repository: db.Repository(), hookExecutor: &mockHookExecutor{}}
	srv := newTestGraphQLServer(r)
	srv.AroundFields(r.LibraryLimitMiddleware)
	// the loaders read custom_fields back from the database
	gql := loaders.Middleware{Repository: r.repository}.Middleware(srv)

	query := fmt.Sprintf(`mutation($cf: Map) { performerUpdate(input:{id:"%d",custom_fields:{full:$cf}}) { custom_fields } }`, performer.ID)
	stored := func() map[string]interface{} {
		var ret map[string]interface{}
		require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
			var err error
			ret, err = db.Performer().GetCustomFields(ctx, performer.ID)
			return err
		}))
		return ret
	}

	body := customFieldsRequest(t, gql, user, query, `{"cf":{"big":1e20,"beyond":99999999999999999999,"small":5,"decimal":1.5}}`)
	require.JSONEq(t, `{"data":{"performerUpdate":{"custom_fields":{"big":1e20,"beyond":1e20,"small":5,"decimal":1.5}}}}`, body)
	require.Equal(t, map[string]interface{}{
		"big":     1e20,
		"beyond":  1e20,
		"small":   int64(5),
		"decimal": 1.5,
	}, stored())

	// too large even for a float: an error, and nothing changes
	body = customFieldsRequest(t, gql, user, query, `{"cf":{"big":1e400}}`)
	require.Contains(t, body, `custom field \"big\": invalid number \"1e400\"`)
	require.Equal(t, 1e20, stored()["big"])
}
