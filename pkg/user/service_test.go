package user

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
)

type mockConfig struct {
	public bool
	guest  bool
}

func (m *mockConfig) GetPublicAccess() bool     { return m.public }
func (m *mockConfig) GetGuestUserEnabled() bool { return m.guest }
func (m *mockConfig) IsNewSystem() bool         { return false }
func (m *mockConfig) SetGuestUserEnabled(enabled bool) {
	m.guest = enabled
}

type mockStore struct {
	users     map[string]*models.User
	passwords map[int]string
	nextID    int
}

func newMockStore() *mockStore {
	return &mockStore{users: make(map[string]*models.User), passwords: make(map[int]string), nextID: 1}
}

func (m *mockStore) All(ctx context.Context) ([]*models.User, error) {
	ret := make([]*models.User, 0, len(m.users))
	for _, u := range m.users {
		ret = append(ret, u)
	}
	return ret, nil
}

func (m *mockStore) Count(ctx context.Context) (int, error) { return len(m.users), nil }

func (m *mockStore) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	if u, ok := m.users[username]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockStore) FindAdminUsers(ctx context.Context) ([]*models.User, error) {
	var ret []*models.User
	for _, u := range m.users {
		if u.Roles.HasRole(models.RoleEnumAdmin) {
			ret = append(ret, u)
		}
	}
	return ret, nil
}

func (m *mockStore) GetPasswordHash(ctx context.Context, id int) (string, error) {
	return m.passwords[id], nil
}

func (m *mockStore) Create(ctx context.Context, u *models.User, password string) error {
	u.ID = m.nextID
	m.nextID++
	m.users[u.Username] = u
	if password != "" {
		m.passwords[u.ID] = password
	}
	return nil
}

func (m *mockStore) Update(ctx context.Context, updated *models.User) error {
	if _, ok := m.users[updated.Username]; !ok {
		// try to find by id and replace key if renamed
		for key, u := range m.users {
			if u.ID == updated.ID {
				delete(m.users, key)
				m.users[updated.Username] = updated
				return nil
			}
		}
		return nil
	}
	m.users[updated.Username] = updated
	return nil
}

func (m *mockStore) SetUserPassword(ctx context.Context, id int, newPassword string) error {
	m.passwords[id] = newPassword
	return nil
}

func (m *mockStore) SetUserAPIKey(ctx context.Context, id int, newAPIKey string) error { return nil }
func (m *mockStore) Destroy(ctx context.Context, id int) error {
	for k, u := range m.users {
		if u.ID == id {
			delete(m.users, k)
			delete(m.passwords, id)
			return nil
		}
	}
	return nil
}

func (m *mockStore) SetLibraryLimit(ctx context.Context, id int, limit *models.LibraryLimit) error {
	for _, u := range m.users {
		if u.ID == id {
			u.LibraryLimit = limit
		}
	}
	return nil
}

func (m *mockStore) SetLock(ctx context.Context, id int, locked bool) error {
	for _, u := range m.users {
		if u.ID == id {
			u.Locked = locked
			return nil
		}
	}
	return nil
}

func TestDeleteUser(t *testing.T) {
	store := newMockStore()
	cfg := &mockConfig{public: false}
	svc := &Service{Store: store, Config: cfg}

	ctx := session.SetCurrentUser(context.Background(), models.User{Username: "admin"})

	// create admin user
	adminUser := &models.User{
		Username: "admin",
		Roles:    models.Roles{models.RoleEnumAdmin},
	}
	if err := svc.CreateUser(ctx, *adminUser, "password"); err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}

	// delete only admin user - should fail since it is the last admin user
	if err := svc.DeleteUser(ctx, "admin"); err == nil {
		t.Fatal("expected error when deleting last admin user, got nil")
	}

	// create non-admin user
	nonAdminUser := &models.User{
		Username: "user1",
		Roles:    models.Roles{models.RoleEnumRead},
	}
	if err := svc.CreateUser(ctx, *nonAdminUser, "password"); err != nil {
		t.Fatalf("failed to create non-admin user: %v", err)
	}

	// delete admin user - should still fail
	if err := svc.DeleteUser(ctx, "admin"); err == nil {
		t.Fatal("expected error when deleting admin user, got nil")
	}

	// delete non-admin user - should succeed
	if err := svc.DeleteUser(ctx, "user1"); err != nil {
		t.Fatalf("failed to delete non-admin user: %v", err)
	}
}

func TestLockedAdminDoesNotCountAsRemainingAdmin(t *testing.T) {
	store := newMockStore()
	svc := &Service{Store: store, Config: &mockConfig{}}
	ctx := session.SetCurrentUser(context.Background(), models.User{Username: "first"})

	for _, name := range []string{"first", "second"} {
		u := models.User{Username: name, Roles: models.Roles{models.RoleEnumAdmin}}
		if err := svc.CreateUser(ctx, u, "password"); err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
	}

	if err := svc.LockUser(ctx, "second"); err != nil {
		t.Fatalf("failed to lock second admin: %v", err)
	}

	if err := svc.UpdateUser(ctx, "first", models.User{Username: "first", Roles: models.Roles{models.RoleEnumRead}}); !errors.Is(err, ErrRemoveLastAdminRole) {
		t.Fatalf("expected %v removing the role from the only usable admin, got %v", ErrRemoveLastAdminRole, err)
	}

	if err := svc.DeleteUser(ctx, "first"); !errors.Is(err, ErrDeleteLastAdminUser) {
		t.Fatalf("expected %v deleting the only usable admin, got %v", ErrDeleteLastAdminUser, err)
	}

	if err := svc.UnlockUser(ctx, "second"); err != nil {
		t.Fatalf("failed to unlock second admin: %v", err)
	}

	if err := svc.DeleteUser(ctx, "first"); err != nil {
		t.Fatalf("failed to delete admin once another is usable: %v", err)
	}
}

func TestGuestUsernameIsReserved(t *testing.T) {
	store := newMockStore()
	svc := &Service{Store: store, Config: &mockConfig{}}
	ctx := context.Background()

	admin := models.User{Username: "admin", Roles: models.Roles{models.RoleEnumAdmin}}
	if err := svc.CreateUser(ctx, admin, "password"); err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}

	guest := models.User{Username: "Guest", Roles: models.Roles{models.RoleEnumRead}}
	if err := svc.CreateUser(ctx, guest, "password"); !errors.Is(err, ErrUsernameReserved) {
		t.Fatalf("expected %v creating a guest account, got %v", ErrUsernameReserved, err)
	}

	renamed := models.User{Username: "guest", Roles: models.Roles{models.RoleEnumAdmin}}
	if err := svc.UpdateUser(ctx, "admin", renamed); !errors.Is(err, ErrUsernameReserved) {
		t.Fatalf("expected %v renaming to guest, got %v", ErrUsernameReserved, err)
	}
}

func TestUpdateUserEndsSessionsOnlyForNameOrRoles(t *testing.T) {
	store := newMockStore()
	svc := &Service{Store: store, Config: &mockConfig{}}
	ctx := context.Background()

	admin := models.User{Username: "admin", Roles: models.Roles{models.RoleEnumAdmin}}
	if err := svc.CreateUser(ctx, admin, "password"); err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}
	listener := models.User{Username: "listener", Roles: models.Roles{models.RoleEnumRead}}
	if err := svc.CreateUser(ctx, listener, "password"); err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	loginTime := time.Now().Add(time.Hour)
	before := store.users["listener"].UpdatedAt

	if err := svc.UpdateUser(ctx, "listener", models.User{Username: "listener", Notes: "edited", Roles: models.Roles{models.RoleEnumRead}}); err != nil {
		t.Fatalf("failed to edit notes: %v", err)
	}
	if got := store.users["listener"].UpdatedAt; !got.Equal(before) {
		t.Fatalf("editing notes changed the session timestamp from %v to %v", before, got)
	}

	store.users["listener"].UpdatedAt = loginTime.Add(-2 * time.Hour)
	if err := svc.UpdateUser(ctx, "listener", models.User{Username: "listener", Roles: models.Roles{models.RoleEnumModify}}); err != nil {
		t.Fatalf("failed to change roles: %v", err)
	}
	if got := store.users["listener"].UpdatedAt; !got.After(loginTime.Add(-2 * time.Hour)) {
		t.Fatalf("changing roles did not move the session timestamp: %v", got)
	}
}

func TestLibraryLimitOnlyForReadOnlyAccounts(t *testing.T) {
	store := newMockStore()
	svc := &Service{Store: store, Config: &mockConfig{}}
	ctx := context.Background()
	limit := &models.LibraryLimit{TagIDs: []int{3, 1, 3}}

	if err := svc.CreateUser(ctx, models.User{Username: "admin", Roles: models.Roles{models.RoleEnumAdmin}}, "password"); err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}

	for _, role := range []models.RoleEnum{models.RoleEnumAdmin, models.RoleEnumModify} {
		err := svc.CreateUser(ctx, models.User{Username: "limited-" + string(role), Roles: models.Roles{role}, LibraryLimit: limit}, "password")
		if !errors.Is(err, ErrLibraryLimitRole) {
			t.Fatalf("creating %s with a limit: got %v", role, err)
		}
	}

	if err := svc.CreateUser(ctx, models.User{Username: "reader", Roles: models.Roles{models.RoleEnumRead}, LibraryLimit: limit}, "password"); err != nil {
		t.Fatalf("failed to create limited reader: %v", err)
	}

	if err := svc.UpdateUser(ctx, "reader", models.User{Username: "reader", Roles: models.Roles{models.RoleEnumModify}}); !errors.Is(err, ErrLibraryLimitRole) {
		t.Fatalf("giving a limited account the modify role: got %v", err)
	}

	if err := svc.SetLibraryLimit(ctx, "admin", limit); !errors.Is(err, ErrLibraryLimitRole) {
		t.Fatalf("limiting an admin: got %v", err)
	}

	if err := svc.SetLibraryLimit(ctx, "missing", limit); !errors.Is(err, ErrUserNotExist) {
		t.Fatalf("limiting a missing account: got %v", err)
	}

	if err := svc.SetLibraryLimit(ctx, "reader", limit); err != nil {
		t.Fatalf("failed to limit reader: %v", err)
	}
	if got := store.users["reader"].LibraryLimit.TagIDs; !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("limit wasn't normalised: %v", got)
	}

	if err := svc.SetLibraryLimit(ctx, "reader", nil); err != nil {
		t.Fatalf("failed to clear limit: %v", err)
	}
	if store.users["reader"].LibraryLimit != nil {
		t.Fatal("limit wasn't cleared")
	}
	if err := svc.UpdateUser(ctx, "reader", models.User{Username: "reader", Roles: models.Roles{models.RoleEnumModify}}); err != nil {
		t.Fatalf("giving an unlimited account the modify role: %v", err)
	}
}
