package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/require"
)

func videoUploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	operations, err := json.Marshal(map[string]any{
		"query":     `mutation($file:Upload!){sceneUpload(file:$file,library_path:"not-a-library"){id}}`,
		"variables": map[string]any{"file": nil},
	})
	require.NoError(t, err)
	require.NoError(t, w.WriteField("operations", string(operations)))
	require.NoError(t, w.WriteField("map", `{"0":["variables.file"]}`))
	f, err := w.CreateFormFile("0", "video.mp4")
	require.NoError(t, err)
	_, err = f.Write([]byte("video"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/graphql", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestSceneUploadNeedsAdmin(t *testing.T) {
	r := newResolver(mocks.NewDatabase())
	schema := NewExecutableSchema(Config{Resolvers: r}).Schema()
	field := schema.Mutation.Fields.ForName("sceneUpload")
	require.NotNil(t, field)
	directive := field.Directives.ForName("hasRole")
	require.NotNil(t, directive)
	require.Equal(t, "ADMIN", directive.Arguments.ForName("role").Value.Raw)
	require.Equal(t, "Upload", field.Arguments.ForName("file").Type.Name())

	srv := newTestGraphQLServer(r)
	srv.AddTransport(transport.MultipartForm{})
	srv.AroundFields(r.LibraryLimitMiddleware)
	for _, role := range []models.RoleEnum{models.RoleEnumRead, models.RoleEnumModify} {
		req := videoUploadRequest(t)
		req = req.WithContext(session.SetCurrentUser(req.Context(), models.User{ID: 2, Roles: models.Roles{role}}))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		require.Contains(t, w.Body.String(), "unauthorized")
		require.NotContains(t, w.Body.String(), "not-a-library")
	}
}

func TestSceneUploadMultipartSizeLimit(t *testing.T) {
	srv := newTestGraphQLServer(newResolver(mocks.NewDatabase()))
	srv.AddTransport(transport.MultipartForm{MaxUploadSize: 1})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, videoUploadRequest(t))
	require.Contains(t, w.Body.String(), "request body too large")
}
