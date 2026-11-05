package user

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

// Replacing a key twice quickly must give a new key each time, and the
// replaced key must stop working (#2136).
func TestReplacedAPIKeyStopsWorking(t *testing.T) {
	config.InitializeEmpty().SetString(config.JWTSignKey, "user-apikey-replace-test-key")
	ctx := context.Background()
	store := newMockStore()
	hash, err := hashPassword("password")
	require.NoError(t, err)
	u := &models.User{ID: 1, Username: "reader", Roles: models.Roles{models.RoleEnumRead}}
	store.users[u.Username] = u
	store.passwords[u.ID] = hash
	s := &Service{Store: store, Config: &mockConfig{}}
	require.NoError(t, s.Init(ctx))

	first, err := s.GenerateAPIKey(ctx, u.Username)
	require.NoError(t, err)
	second, err := s.GenerateAPIKey(ctx, u.Username)
	require.NoError(t, err)
	require.NotEqual(t, first, second, "keys made back to back differ")

	_, err = s.AuthenticateByAPIKey(ctx, first)
	require.ErrorIs(t, err, ErrAccessDenied, "replaced key is refused")
	keyUser, err := s.AuthenticateByAPIKey(ctx, second)
	require.NoError(t, err)
	require.Equal(t, u.ID, keyUser.ID)

	// the key still names its user and is checked the same way
	userID, err := GetUserIDFromAPIKey(second)
	require.NoError(t, err)
	require.Equal(t, u.Username, userID)
}

// Keys made before keys had a random ID must keep working (#2136).
func TestAPIKeyWithoutIDStillWorks(t *testing.T) {
	config.InitializeEmpty().SetString(config.JWTSignKey, "user-apikey-old-test-key")
	ctx := context.Background()
	store := newMockStore()
	hash, err := hashPassword("password")
	require.NoError(t, err)
	u := &models.User{ID: 1, Username: "reader", Roles: models.Roles{models.RoleEnumRead}}
	store.users[u.Username] = u
	store.passwords[u.ID] = hash
	s := &Service{Store: store, Config: &mockConfig{}}
	require.NoError(t, s.Init(ctx))

	// the format keys had before: user name, subject and issue time only
	claims := &APIKeyClaims{UserID: u.Username, RegisteredClaims: jwt.RegisteredClaims{
		Subject:  APIKeySubject,
		IssuedAt: jwt.NewNumericDate(time.Unix(1700000000, 0)),
	}}
	oldKey, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(config.GetInstance().GetJWTSignKey())
	require.NoError(t, err)
	u.ApiKeyHash, err = HashAPIKey(oldKey)
	require.NoError(t, err)

	keyUser, err := s.AuthenticateByAPIKey(ctx, oldKey)
	require.NoError(t, err, "old key still works")
	require.Equal(t, u.ID, keyUser.ID)
}

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
