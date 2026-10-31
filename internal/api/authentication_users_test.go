package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type apiTestUsers struct {
	manager.UserService
	keys  map[string]*models.User
	guest *models.User
}

func (*apiTestUsers) IsSingleUserMode() bool                              { return false }
func (*apiTestUsers) GetSingleUser(context.Context) (*models.User, error) { return nil, nil }
func (s *apiTestUsers) GetGuestUser(context.Context) *models.User         { return s.guest }
func (s *apiTestUsers) AllUsers(context.Context) ([]*models.User, error) {
	return []*models.User{s.keys["admin-key"], s.keys["read-key"]}, nil
}
func (s *apiTestUsers) AuthenticateByAPIKey(_ context.Context, key string) (*models.User, error) {
	if u := s.keys[key]; u != nil {
		return u, nil
	}
	return nil, user.ErrAccessDenied
}
func (s *apiTestUsers) AuthenticateSession(_ context.Context, username string, _ time.Time) (*models.User, error) {
	for _, u := range s.keys {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, user.ErrAccessDenied
}
func (s *apiTestUsers) ValidateCredentials(_ context.Context, username, password string) error {
	if username == "reader" && password == "reader-password" {
		return nil
	}
	return user.ErrAccessDenied
}

func TestMultiUserHTTPPermissions(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetPublicAccess(true)
	cfg.SetString(config.SessionStoreKey, "authentication-test-cookie-secret")
	cfg.SetInt(config.MaxSessionAge, 3600)
	db := mocks.NewDatabase()
	tx := db.Repository().TxnManager
	users := &apiTestUsers{keys: map[string]*models.User{
		"admin-key":  {ID: 1, Username: "admin", Roles: models.Roles{models.RoleEnumAdmin}},
		"modify-key": {ID: 2, Username: "editor", Roles: models.Roles{models.RoleEnumModify}},
		"read-key":   {ID: 3, Username: "reader", Roles: models.Roles{models.RoleEnumRead}},
	}}
	sessions := session.NewStore(cfg, users, &tx)
	r := newResolver(db)
	r.userService = users
	r.repository.VoiceActor = &apiVoiceActorStore{}
	gql := handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: r, Directives: DirectiveRoot{HasRole: HasRoleDirective, IsUserOwner: IsUserOwnerDirective}}))
	h := authenticateHandlerWithServices(tx, cfg, func() UserAuthenticator { return users }, sessions)(gql)
	request := func(query, key string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]string{"query": query})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set(session.ApiKeyHeader, key)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		req = req.WithContext(context.WithValue(req.Context(), IPCtxKey, net.ParseIP("127.0.0.1")))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	me := `{ me { name roles } }`
	require.Equal(t, http.StatusUnauthorized, request(me, "", nil).Code)
	require.Equal(t, http.StatusUnauthorized, request(me, "invalid-key", nil).Code)
	w := request(me, "read-key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"name":"reader"`)
	for _, mutation := range []string{
		`voiceActorCreate(input:{name:"Voice"}) { id name }`,
		`voiceActorUpdate(input:{id:"42",name:"Changed"}) { id }`,
		`voiceActorDestroy(id:"42")`,
		`fileSetVRMetadata(input:{id:"1"})`,
		`fileSetFingerprints(input:{id:"1",fingerprints:[]})`,
		`scanFile(input:{path:"example.mp4"}) { id }`,
		`verifyPaths(input:{})`,
		`purgeMissing(input:{})`,
		`audioCreate(input:{title:"Audio"}) { id }`,
		`audioUpdate(input:{id:"1",title:"Changed"}) { id }`,
		`audioMerge(input:{source:["1"],destination:"2"}) { id }`,
		`bulkAudioUpdate(input:{ids:["1"],title:"Changed"}) { id }`,
		`audiosDestroy(input:{ids:["1"]})`,
		`audiosUpdate(input:[{id:"1",title:"Changed"}]) { id }`,
		`audioAssignFile(input:{audio_id:"1",file_id:"1"})`,
		`backupDatabase(input:{})`,
		`submitStashBoxFingerprints(input:{scene_ids:[],stash_box_endpoint:"https://example.invalid/graphql"})`,
	} {
		t.Run(mutation, func(t *testing.T) {
			w := request("mutation { "+mutation+" }", "read-key", nil)
			require.Contains(t, w.Body.String(), "unauthorized", "permission must reject before resolving mutation")
		})
	}
	for _, key := range []string{"modify-key", "admin-key"} {
		w := request(`mutation { voiceActorCreate(input:{name:"Voice"}) { id name } }`, key, nil)
		require.Contains(t, w.Body.String(), `"id":"42"`)
		require.NotContains(t, w.Body.String(), `"errors"`)
	}
	db.Audio.On("AddO", mock.Anything, 42, []time.Time(nil)).Return([]time.Time{}, nil).Once()
	w = request(`mutation { audioAddO(id:"42") { count } }`, "read-key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "unauthorized")
	require.NotContains(t, w.Body.String(), `"errors"`)

	db.Audio.On("SaveActivity", mock.Anything, 42, mock.Anything, mock.Anything).Return(true, nil).Once()
	w = request(`mutation { audioSaveActivity(id:"42",resume_time:12.5,playDuration:3.2) }`, "read-key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "unauthorized")
	require.NotContains(t, w.Body.String(), `"errors"`)
	// a read-only account can set its own rating, without that counting as editing the track
	db.Audio.On("UpdatePartial", mock.Anything, 42, mock.MatchedBy(func(p models.AudioPartial) bool {
		return p.Rating.Set && p.Rating.Value == 60 && !p.UpdatedAt.Set && !p.Title.Set
	})).Return(&models.Audio{ID: 42}, nil).Once()
	db.Audio.On("Find", mock.Anything, 42).Return(&models.Audio{ID: 42}, nil).Once()
	w = request(`mutation { audioUserDataUpdate(input:{id:"42",rating100:60}) { id } }`, "read-key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "unauthorized")
	require.NotContains(t, w.Body.String(), `"errors"`)
	w = request(`mutation { audioUpdate(input:{id:"42",rating100:60}) { id } }`, "read-key", nil)
	require.Contains(t, w.Body.String(), "unauthorized")
	db.Audio.AssertExpectations(t)
	w = request(`{ users { name } }`, "read-key", nil)
	require.Contains(t, w.Body.String(), "unauthorized")
	w = request(`{ users { name } }`, "admin-key", nil)
	require.NotContains(t, w.Body.String(), `"errors"`)
	require.Contains(t, w.Body.String(), `"name":"admin"`)
	login := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=reader&password=reader-password"))
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResponse := httptest.NewRecorder()
	require.NoError(t, sessions.Login(loginResponse, login))
	cookies := loginResponse.Result().Cookies()
	require.NotEmpty(t, cookies)
	w = request(me, "", cookies[0])
	require.Contains(t, w.Body.String(), `"name":"reader"`)
	users.guest = &models.User{Username: "guest", Roles: models.Roles{models.RoleEnumRead}}
	w = request(me, "", nil)
	require.Contains(t, w.Body.String(), `"name":"guest"`)
	w = request(`mutation { voiceActorDestroy(id:"42") }`, "", nil)
	require.Contains(t, w.Body.String(), "unauthorized")
}
