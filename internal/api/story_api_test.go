package api

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"
)

func storyRequest(t *testing.T, gql *handler.Server, u models.User, query string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(session.SetCurrentUser(req.Context(), u))
	w := httptest.NewRecorder()
	gql.ServeHTTP(w, req)
	return w.Body.String()
}

func TestStoryRoles(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)

	story := &models.Story{ID: 1, Title: "A story"}
	db.Story.On("Find", mock.Anything, 1).Return(story, nil)
	db.Story.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(story, nil)
	db.Story.On("Destroy", mock.Anything, 1).Return(nil)

	reader := models.User{ID: 2, Username: "reader", Roles: models.Roles{models.RoleEnumRead}}
	editor := models.User{ID: 3, Username: "editor", Roles: models.Roles{models.RoleEnumModify}}

	body := storyRequest(t, gql, reader, `{ findStory(id:"1") { id title } }`)
	require.Contains(t, body, `"title":"A story"`)

	// rating is the account's own, so reading is enough
	body = storyRequest(t, gql, reader, `mutation { storyUserDataUpdate(input:{id:"1",rating100:60}) { id } }`)
	require.NotContains(t, body, "errors")

	changes := []string{
		`storyCreate(input:{title:"new"}) { id }`,
		`storyUpdate(input:{id:"1",title:"changed"}) { id }`,
		`storiesDestroy(input:{ids:["1"]})`,
	}
	for _, mutation := range changes {
		body := storyRequest(t, gql, reader, "mutation { "+mutation+" }")
		require.Contains(t, body, "unauthorized", mutation)
	}

	body = storyRequest(t, gql, editor, `mutation { storyUpdate(input:{id:"1",title:"changed"}) { id } }`)
	require.NotContains(t, body, "errors")
	body = storyRequest(t, gql, editor, `mutation { storiesDestroy(input:{ids:["1"]}) }`)
	require.Contains(t, body, `"storiesDestroy":true`)
}

func TestStoryLibraryLimit(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	gql.AroundFields(r.LibraryLimitMiddleware)

	// the store leaves out what the limit hides
	const visibleID, hiddenID = 1, 2
	visible := &models.Story{ID: visibleID, Title: "visible"}
	db.Story.On("Find", mock.Anything, visibleID).Return(visible, nil)
	db.Story.On("Find", mock.Anything, hiddenID).Return(nil, nil)
	db.Story.On("UpdatePartial", mock.Anything, visibleID, mock.Anything).Return(visible, nil)

	limit := &models.LibraryLimit{TagIDs: []int{7}}
	reader := models.User{ID: 3, Username: "reader", Roles: models.Roles{models.RoleEnumRead}, LibraryLimit: limit}
	editor := models.User{ID: 4, Username: "editor", Roles: models.Roles{models.RoleEnumModify}, LibraryLimit: limit}

	body := storyRequest(t, gql, reader, `{ findStory(id:"1") { title } }`)
	require.Contains(t, body, `"title":"visible"`)
	body = storyRequest(t, gql, reader, `{ findStory(id:"2") { title } }`)
	require.Equal(t, `{"data":{"findStory":null}}`, body)

	// the same answer as something that doesn't exist
	body = storyRequest(t, gql, reader, `mutation { storyUserDataUpdate(input:{id:"2",rating100:60}) { id } }`)
	require.Contains(t, body, `"message":"not found"`)
	body = storyRequest(t, gql, reader, `mutation { storyUserDataUpdate(input:{id:"1",rating100:60}) { id } }`)
	require.NotContains(t, body, "errors")

	// changing the library is refused, even with permission to edit and for what can be seen
	for _, mutation := range []string{
		`storyCreate(input:{title:"new"}) { id }`,
		`storyUpdate(input:{id:"1",title:"changed"}) { id }`,
		`storiesDestroy(input:{ids:["1"]})`,
	} {
		body := storyRequest(t, gql, editor, "mutation { "+mutation+" }")
		require.Contains(t, body, "unauthorized", mutation)
	}
}

func TestStoryCreateChecksFiles(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	editor := models.User{ID: 3, Username: "editor", Roles: models.Roles{models.RoleEnumModify}}

	root := t.TempDir()
	stories, videos := filepath.Join(root, "stories"), filepath.Join(root, "videos")
	config.InitializeEmpty()
	config.GetInstance().SetInterface(config.Stash, config.StashConfigs{{Path: stories, IncludeStory: true}, {Path: videos}})

	video := &models.VideoFile{BaseFile: &models.BaseFile{ID: 5, Path: filepath.Join(stories, "video.mp4")}}
	used := &models.BaseFile{ID: 6, Path: filepath.Join(stories, "used.txt")}
	inStory := &models.BaseFile{ID: 7, Path: filepath.Join(stories, "other-story.txt")}
	free := &models.BaseFile{ID: 8, Path: filepath.Join(stories, "free.txt")}
	// a zip is a plain file record too, so its extension is what shows it isn't text
	zip := &models.BaseFile{ID: 11, Path: filepath.Join(stories, "pack.zip")}
	readme := &models.BaseFile{ID: 12, Path: filepath.Join(videos, "readme.txt")}
	db.File.On("Find", mock.Anything, models.FileID(11)).Return([]models.File{zip}, nil)
	db.File.On("Find", mock.Anything, models.FileID(12)).Return([]models.File{readme}, nil)
	db.File.On("Find", mock.Anything, models.FileID(5)).Return([]models.File{video}, nil)
	db.File.On("Find", mock.Anything, models.FileID(6)).Return([]models.File{used}, nil)
	db.File.On("Find", mock.Anything, models.FileID(7)).Return([]models.File{inStory}, nil)
	db.File.On("Find", mock.Anything, models.FileID(8)).Return([]models.File{free}, nil)
	db.File.On("IsPrimary", mock.Anything, models.FileID(6)).Return(true, nil)
	db.File.On("IsPrimary", mock.Anything, models.FileID(7)).Return(false, nil)
	db.File.On("IsPrimary", mock.Anything, models.FileID(8)).Return(false, nil)
	db.Story.On("CountByFileID", mock.Anything, models.FileID(7)).Return(1, nil)
	db.Story.On("CountByFileID", mock.Anything, models.FileID(8)).Return(0, nil)

	for query, want := range map[string]string{
		`mutation { storyCreate(input:{}) { id } }`:                          "title must be set",
		`mutation { storyCreate(input:{file_ids:["5"]}) { id } }`:            "is not a text file",
		`mutation { storyCreate(input:{file_ids:["11"]}) { id } }`:           "is not a text file",
		`mutation { storyCreate(input:{file_ids:["12"]}) { id } }`:           "in a library that doesn't include stories",
		`mutation { storyCreate(input:{file_ids:["6"]}) { id } }`:            "already belongs to something else",
		`mutation { storyCreate(input:{file_ids:["7"]}) { id } }`:            "already belongs to another story",
		`mutation { storyCreate(input:{file_ids:["8","8"]}) { id } }`:        "listed twice",
		`mutation { storyCreate(input:{title:"x",file_ids:["9x"]}) { id } }`: "converting file ids",
	} {
		body := storyRequest(t, gql, editor, query)
		require.Contains(t, body, want, query)
	}
	db.Story.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything)

	db.Story.On("Create", mock.Anything, mock.Anything, []models.FileID{8}).Run(func(args mock.Arguments) {
		args.Get(1).(*models.Story).ID = 10
	}).Return(nil)
	db.Story.On("Find", mock.Anything, 10).Return(&models.Story{ID: 10, Title: "new"}, nil)

	body := storyRequest(t, gql, editor, `mutation { storyCreate(input:{title:"new",author:"someone",file_ids:["8"]}) { id title } }`)
	require.Equal(t, `{"data":{"storyCreate":{"id":"10","title":"new"}}}`, body)

	created := db.Story.Calls[len(db.Story.Calls)-2].Arguments.Get(1).(*models.Story)
	require.Equal(t, "someone", created.Author)
}

func TestStoryUpdateChecksPrimaryFile(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	editor := models.User{ID: 3, Username: "editor", Roles: models.Roles{models.RoleEnumModify}}

	db.Story.On("Find", mock.Anything, 1).Return(&models.Story{ID: 1, Title: "A story"}, nil)
	db.Story.On("GetFiles", mock.Anything, 1).Return([]models.File{&models.BaseFile{ID: 5}}, nil)

	body := storyRequest(t, gql, editor, `mutation { storyUpdate(input:{id:"1",primary_file_id:"6"}) { id } }`)
	require.Contains(t, body, "not associated with story")

	db.Story.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(&models.Story{ID: 1}, nil)
	body = storyRequest(t, gql, editor, `mutation { storyUpdate(input:{id:"1",primary_file_id:"5"}) { id } }`)
	require.NotContains(t, body, "errors")
}

func TestStoriesDestroyKeepsFile(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	gql := newTestGraphQLServer(r)
	editor := models.User{ID: 3, Username: "editor", Roles: models.Roles{models.RoleEnumModify}}

	path := filepath.Join(t.TempDir(), "story.txt")
	require.NoError(t, os.WriteFile(path, []byte("Once upon a time"), 0o644))

	db.Story.On("Find", mock.Anything, 1).Return(&models.Story{ID: 1, Path: path}, nil)
	db.Story.On("Destroy", mock.Anything, 1).Return(nil)
	db.Story.On("Find", mock.Anything, 2).Return(nil, nil)

	// the file mock has no expectations, so any change to files would fail
	body := storyRequest(t, gql, editor, `mutation { storiesDestroy(input:{ids:["1"]}) }`)
	require.Equal(t, `{"data":{"storiesDestroy":true}}`, body)
	require.FileExists(t, path)
	db.File.AssertExpectations(t)

	body = storyRequest(t, gql, editor, `mutation { storiesDestroy(input:{ids:["2"]}) }`)
	require.Contains(t, body, "not found")
}

func TestStoryTextRoute(t *testing.T) {
	db := mocks.NewDatabase()
	rs := storyRoutes{routes: routes{txnManager: db.Repository().TxnManager}, storyFinder: db.Story, fileGetter: db.File}
	router := rs.Routes()

	// HTML in a story file is shown as text, never run
	const text = "<script>alert(1)</script>\nOnce upon a time"
	path := filepath.Join(t.TempDir(), "story.html")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o644))

	fileID := models.FileID(5)
	db.Story.On("Find", mock.Anything, 1).Return(&models.Story{ID: 1, PrimaryFileID: &fileID}, nil)
	db.Story.On("Find", mock.Anything, 2).Return(nil, nil)
	db.Story.On("Find", mock.Anything, 3).Return(&models.Story{ID: 3}, nil)
	db.File.On("Find", mock.Anything, fileID).Return([]models.File{&models.BaseFile{ID: fileID, Path: path, Basename: "story.html", DirEntry: models.DirEntry{ModTime: time.Now()}}}, nil)

	get := func(url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		return w
	}

	w := get("/1/text")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, text, w.Body.String())
	require.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "sandbox", w.Header().Get("Content-Security-Policy"))

	// a story that's hidden or doesn't exist, and one without a file, all get the same answer
	missing := get("/2/text")
	require.Equal(t, http.StatusNotFound, missing.Code)
	noFile := get("/3/text")
	require.Equal(t, missing.Code, noFile.Code)
	require.Equal(t, missing.Body.String(), noFile.Body.String())

	require.Equal(t, http.StatusBadRequest, get("/x/text").Code)
}

// A story saved in an older encoding, on disk or inside a zip, is sent as
// UTF-8, and a range of it is a range of the UTF-8 text.
func TestStoryTextRouteDecodes(t *testing.T) {
	db := mocks.NewDatabase()
	rs := storyRoutes{routes: routes{txnManager: db.Repository().TxnManager}, storyFinder: db.Story, fileGetter: db.File}
	router := rs.Routes()

	const text = "Il était une fois, dans un café près de la forêt, une jeune fille qui rêvait d'été."
	latin1, err := charmap.ISO8859_1.NewEncoder().Bytes([]byte(text))
	require.NoError(t, err)

	dir := t.TempDir()
	path := filepath.Join(dir, "story.txt")
	require.NoError(t, os.WriteFile(path, latin1, 0o644))

	zipPath := filepath.Join(dir, "stories.zip")
	zf, err := os.Create(zipPath)
	require.NoError(t, err)
	zw := zip.NewWriter(zf)
	entry, err := zw.Create("inside.txt")
	require.NoError(t, err)
	_, err = entry.Write(latin1)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, zf.Close())
	zipInfo, err := os.Stat(zipPath)
	require.NoError(t, err)

	onDisk, inZip := models.FileID(5), models.FileID(6)
	modTime := models.DirEntry{ModTime: time.Now()}

	db.Story.On("Find", mock.Anything, 1).Return(&models.Story{ID: 1, PrimaryFileID: &onDisk}, nil)
	db.Story.On("Find", mock.Anything, 2).Return(&models.Story{ID: 2, PrimaryFileID: &inZip}, nil)
	db.File.On("Find", mock.Anything, onDisk).Return([]models.File{&models.BaseFile{ID: onDisk, Path: path, Basename: "story.txt", Size: int64(len(latin1)), DirEntry: modTime}}, nil)
	db.File.On("Find", mock.Anything, inZip).Return([]models.File{&models.BaseFile{
		ID: inZip, Path: filepath.Join(zipPath, "inside.txt"), Basename: "inside.txt", Size: int64(len(latin1)), DirEntry: models.DirEntry{ModTime: time.Now(),
			ZipFile: &models.BaseFile{Path: zipPath, Basename: "stories.zip", Size: zipInfo.Size()}},
	}}, nil)

	for _, url := range []string{"/1/text", "/2/text"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		require.Equal(t, http.StatusOK, w.Code, url)
		require.Equal(t, text, w.Body.String(), url)
		require.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"), url)
	}

	req := httptest.NewRequest(http.MethodGet, "/1/text", nil)
	req.Header.Set("Range", "bytes=3-11")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusPartialContent, w.Code)
	require.Equal(t, text[3:12], w.Body.String())
}

func TestStoryTextURL(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	paths, err := r.Story().Paths(testCtx, &models.Story{ID: 4})
	require.NoError(t, err)
	require.Equal(t, "/story/4/text", *paths.Text)
}
