package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestLibraryLimitListsMatchSchema(t *testing.T) {
	schema := NewExecutableSchema(Config{Resolvers: newResolver(mocks.NewDatabase())}).Schema()

	for name := range limitedQueries {
		require.NotNil(t, schema.Query.Fields.ForName(name), name)
	}
	for name := range limitedAccountMutations {
		require.NotNil(t, schema.Mutation.Fields.ForName(name), name)
	}
	for name := range limitedFolderFields {
		require.NotNil(t, schema.Types["Folder"].Fields.ForName(name), name)
	}

	// a mutation without an object to check would always be refused
	inputs := map[string]bool{"UserRatingInput": true, "UserRatingFavoriteInput": true, "UserFavoriteInput": true}
	for name := range limitedObjectMutations {
		field := schema.Mutation.Fields.ForName(name)
		require.NotNil(t, field, name)
		if id := field.Arguments.ForName("id"); id != nil {
			require.Equal(t, "ID", id.Type.Name(), name)
			continue
		}
		input := field.Arguments.ForName("input")
		require.NotNil(t, input, name)
		require.True(t, inputs[input.Type.Name()], "%s takes %s", name, input.Type.Name())
	}
}

func TestLibraryLimitRefusesWhatItDoesNotKnow(t *testing.T) {
	r := newResolver(mocks.NewDatabase())
	limited := models.WithLibraryLimit(context.Background(), &models.LibraryLimit{})
	called := false
	next := func(context.Context) (any, error) {
		called = true
		return "result", nil
	}
	call := func(ctx context.Context, object, field string, args map[string]any) (any, error) {
		called = false
		return r.LibraryLimitMiddleware(graphql.WithFieldContext(ctx, &graphql.FieldContext{
			Object: object,
			Field:  graphql.CollectedField{Field: &ast.Field{Name: field}},
			Args:   args,
		}), next)
	}

	for _, tt := range []struct{ object, field string }{
		{"Query", "findFile"},
		{"Query", "findFiles"},
		{"Query", "findFolder"},
		{"Query", "findFolders"},
		{"Query", "findDuplicateScenes"},
		{"Query", "markerStrings"},
		{"Query", "pluginTasks"},
		{"Query", "directory"},
		{"Query", "somethingAddedLater"},
		{"Mutation", "sceneUpdate"},
		{"Mutation", "somethingAddedLater"},
		{"Subscription", "jobsSubscribe"},
		{"Subscription", "scanCompleteSubscribe"},
		{"Subscription", "somethingAddedLater"},
		{"Folder", "sub_folders"},
	} {
		_, err := call(limited, tt.object, tt.field, nil)
		require.ErrorIs(t, err, session.ErrUnauthorized, tt.field)
		require.False(t, called, tt.field)

		_, err = call(context.Background(), tt.object, tt.field, nil)
		require.NoError(t, err, tt.field)
		require.True(t, called, "%s without a limit", tt.field)
	}

	// polled by the interface, and there's nothing to show
	ret, err := call(limited, "Query", "jobQueue", nil)
	require.NoError(t, err)
	require.Nil(t, ret)
	require.False(t, called)

	// a mutation that can't name its object isn't let through
	_, err = call(limited, "Mutation", "sceneAddO", map[string]any{"id": "not a number"})
	require.ErrorIs(t, err, session.ErrUnauthorized)
	_, err = call(limited, "Mutation", "sceneAddO", nil)
	require.ErrorIs(t, err, session.ErrUnauthorized)

	// fields of objects that may be seen are for the stores
	_, err = call(limited, "Scene", "title", nil)
	require.NoError(t, err)
	require.True(t, called)
	_, err = call(limited, "Query", "__schema", nil)
	require.NoError(t, err)
}

func TestLibraryLimitOverHTTP(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: r, Directives: DirectiveRoot{HasRole: HasRoleDirective, IsUserOwner: IsUserOwnerDirective}}))
	gql.AroundFields(r.LibraryLimitMiddleware)

	const visibleID, hiddenID = 1, 2
	db.Scene.On("Find", mock.Anything, visibleID).Return(&models.Scene{ID: visibleID}, nil)
	db.Scene.On("Find", mock.Anything, hiddenID).Return(nil, nil)
	db.Audio.On("Find", mock.Anything, hiddenID).Return(nil, nil)

	limit := &models.LibraryLimit{TagIDs: []int{7}}
	reader := models.User{ID: 3, Username: "reader", Roles: models.Roles{models.RoleEnumRead}, LibraryLimit: limit}
	request := func(query string) string {
		t.Helper()
		body, err := json.Marshal(map[string]string{"query": query})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(session.SetCurrentUser(req.Context(), reader))
		w := httptest.NewRecorder()
		gql.ServeHTTP(w, req)
		return w.Body.String()
	}

	// the same answer as something that doesn't exist, and not unauthorized
	for _, mutation := range []string{
		`sceneAddO(id:"2") { count }`,
		`audioAddPlay(id:"2") { count }`,
		`sceneSaveActivity(id:"2",resume_time:1)`,
		`sceneUserDataUpdate(input:{id:"2",rating100:60}) { id }`,
	} {
		body := request("mutation { " + mutation + " }")
		require.Contains(t, body, `"message":"not found"`, mutation)
	}

	db.Scene.On("AddO", mock.Anything, visibleID, []time.Time(nil)).Maybe().Return([]time.Time{}, nil)
	body := request(`mutation { sceneAddO(id:"1") { count } }`)
	require.NotContains(t, body, "not found")
	require.NotContains(t, body, "unauthorized")

	// changing the library is refused, even for what can be seen
	body = request(`mutation { sceneUpdate(input:{id:"1",title:"changed"}) { id } }`)
	require.Contains(t, body, "unauthorized")

	body = request(`{ findFolders { count } }`)
	require.Contains(t, body, "unauthorized")

	// an account can read its own limit, which it needs to know what it's looking at
	body = request(`{ me { name library_limit { tag_ids folder_ids studio_ids } } }`)
	require.Contains(t, body, `"tag_ids":["7"]`)
}

func TestUserSetLibraryLimitNeedsAdmin(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: r, Directives: DirectiveRoot{HasRole: HasRoleDirective, IsUserOwner: IsUserOwnerDirective}}))
	gql.AroundFields(r.LibraryLimitMiddleware)

	for _, u := range []models.User{
		{ID: 2, Username: "editor", Roles: models.Roles{models.RoleEnumModify}},
		{ID: 3, Username: "reader", Roles: models.Roles{models.RoleEnumRead}},
	} {
		body, err := json.Marshal(map[string]string{"query": `mutation { userSetLibraryLimit(input:{name:"reader"}) { name } }`})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(session.SetCurrentUser(req.Context(), u))
		w := httptest.NewRecorder()
		gql.ServeHTTP(w, req)
		require.Contains(t, w.Body.String(), "unauthorized", u.Username)
	}
}

func TestSceneHashNeedsAVisibleScene(t *testing.T) {
	db := mocks.NewDatabase()
	rs := sceneRoutes{routes: routes{txnManager: db.Repository().TxnManager}, sceneFinder: db.Scene}

	request := func(limit *models.LibraryLimit) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/scene/hash_thumbs.vtt", nil)
		return req.WithContext(models.WithLibraryLimit(req.Context(), limit))
	}

	// without a limit nothing is looked up, as the mock would fail on an unexpected call
	require.True(t, rs.hashAllowed(request(nil), "hash"))

	limit := &models.LibraryLimit{TagIDs: []int{1}}
	db.Scene.On("FindByOSHash", mock.Anything, "hidden").Return([]*models.Scene{}, nil)
	db.Scene.On("FindByChecksum", mock.Anything, "hidden").Return([]*models.Scene{}, nil)
	require.False(t, rs.hashAllowed(request(limit), "hidden"))

	db.Scene.On("FindByOSHash", mock.Anything, "visible").Return([]*models.Scene{{ID: 1}}, nil)
	require.True(t, rs.hashAllowed(request(limit), "visible"))

	db.Scene.On("FindByOSHash", mock.Anything, "md5").Return([]*models.Scene{}, nil)
	db.Scene.On("FindByChecksum", mock.Anything, "md5").Return([]*models.Scene{{ID: 1}}, nil)
	require.True(t, rs.hashAllowed(request(limit), "md5"))
}
