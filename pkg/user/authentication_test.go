package user

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestAuthenticationIsolationAndRevocation(t *testing.T) {
	config.InitializeEmpty().SetString(config.JWTSignKey, "user-authentication-test-key")
	ctx := context.Background()
	store := newMockStore()
	hash, err := hashPassword("password")
	require.NoError(t, err)
	u := &models.User{ID: 1, Username: "reader", Roles: models.Roles{models.RoleEnumRead}, UpdatedAt: time.Now().Add(-time.Minute)}
	store.users[u.Username] = u
	store.passwords[u.ID] = hash
	s := &Service{Store: store, Config: &mockConfig{}}
	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.ValidateCredentials(ctx, "reader", "password"))
	require.ErrorIs(t, s.ValidateCredentials(ctx, "reader", "wrong"), ErrAccessDenied)
	key, err := generateAPIKey(u.Username)
	require.NoError(t, err)
	u.ApiKeyHash, err = HashAPIKey(key)
	require.NoError(t, err)
	keyUser, err := s.AuthenticateByAPIKey(ctx, key)
	require.NoError(t, err)
	require.Equal(t, u.ID, keyUser.ID)
	_, err = s.AuthenticateByAPIKey(ctx, "invalid")
	require.ErrorIs(t, err, ErrAccessDenied)
	u.ApiKeyHash = ""
	_, err = s.AuthenticateByAPIKey(ctx, key)
	require.ErrorIs(t, err, ErrAccessDenied, "cleared API key is denied")
	u.ApiKeyHash, err = HashAPIKey(key)
	require.NoError(t, err)
	u.Locked = true
	_, err = s.AuthenticateByAPIKey(ctx, key)
	require.ErrorIs(t, err, ErrAccessDenied)
	require.ErrorIs(t, s.ValidateCredentials(ctx, "reader", "password"), ErrAccessDenied)
	u.Locked = false
	_, err = s.AuthenticateSession(ctx, "reader", time.Now().Add(time.Second))
	require.NoError(t, err)
	_, err = s.AuthenticateSession(ctx, "reader", time.Now().Add(-time.Hour))
	require.ErrorIs(t, err, ErrAccessDenied)
	u.Roles = nil
	_, err = s.AuthenticateByAPIKey(ctx, key)
	require.ErrorIs(t, err, ErrAccessDenied)
}
