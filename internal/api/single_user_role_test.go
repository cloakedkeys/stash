package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// singleReaderUsers is single-user mode whose one account is stored without
// the admin role.
type singleReaderUsers struct {
	*apiTestUsers
}

func (singleReaderUsers) IsSingleUserMode() bool { return true }
func (singleReaderUsers) GetSingleUser(context.Context) (*models.User, error) {
	return &models.User{ID: 1, Username: "solo", Roles: models.Roles{models.RoleEnumRead}}, nil
}

// In single-user mode every request acts as the single user with the admin
// role, whatever roles the account has stored. The same holds for the
// GraphQL calls JavaScript plugins make in-process.
func TestSingleUserIsAdmin(t *testing.T) {
	users := singleReaderUsers{apiTestUsers: &apiTestUsers{}}
	getUsers := func() UserAuthenticator { return users }
	tx := mocks.NewDatabase().Repository().TxnManager

	var got *models.User
	record := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = session.GetCurrentUser(r.Context())
	})

	t.Run("request", func(t *testing.T) {
		cfg := originTestConfig{Config: config.InitializeEmpty()}
		cfg.SetString(config.SessionStoreKey, "single-user-role-secret")
		sessions := session.NewStore(cfg, users, &tx)
		h := RequestIPMiddleware(authenticateHandlerWithServices(tx, cfg, getUsers, sessions)(record))

		got = nil
		req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		h.ServeHTTP(httptest.NewRecorder(), req)

		require.NotNil(t, got)
		assert.Equal(t, "solo", got.Username)
		assert.Equal(t, models.Roles{models.RoleEnumAdmin}, got.Roles)
	})

	t.Run("JavaScript plugin", func(t *testing.T) {
		h := pluginUserHandler(tx, getUsers)(record)

		for name, ctx := range map[string]context.Context{
			"started by nobody": context.Background(),
			"started by a request": session.SetCurrentUser(context.Background(),
				models.User{ID: 1, Username: "solo", Roles: models.Roles{models.RoleEnumRead}}),
		} {
			got = nil
			req := httptest.NewRequest(http.MethodPost, "/graphql", nil).WithContext(ctx)
			h.ServeHTTP(httptest.NewRecorder(), req)

			require.NotNil(t, got, name)
			assert.Equal(t, "solo", got.Username, name)
			assert.Equal(t, models.Roles{models.RoleEnumAdmin}, got.Roles, name)
		}
	})
}
