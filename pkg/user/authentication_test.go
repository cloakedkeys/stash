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
	// renaming keeps the key working, and someone taking the old name doesn't get it
	delete(store.users, "reader")
	u.Username = "renamed"
	store.users[u.Username] = u
	keyUser, err = s.AuthenticateByAPIKey(ctx, key)
	require.NoError(t, err, "API key works after a rename")
	require.Equal(t, u.ID, keyUser.ID)
	other := &models.User{ID: 2, Username: "reader", Roles: models.Roles{models.RoleEnumRead}}
	store.users[other.Username] = other
	keyUser, err = s.AuthenticateByAPIKey(ctx, key)
	require.NoError(t, err)
	require.Equal(t, u.ID, keyUser.ID, "API key belongs to the renamed user")
	delete(store.users, other.Username)
	delete(store.users, u.Username)
	u.Username = "reader"
	store.users[u.Username] = u
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
