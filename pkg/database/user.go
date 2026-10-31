package database

import (
	"context"
	"github.com/stashapp/stash/pkg/models"
)

type UserStore interface {
	All(ctx context.Context) ([]*models.User, error)
	Count(ctx context.Context) (int, error)
	FindByUsername(ctx context.Context, username string) (*models.User, error)
	FindAdminUsers(ctx context.Context) ([]*models.User, error)

	GetPasswordHash(ctx context.Context, id int) (string, error)

	Create(ctx context.Context, u *models.User, password string) error
	Update(ctx context.Context, updated *models.User) error
	SetUserPassword(ctx context.Context, id int, newPassword string) error
	SetUserAPIKey(ctx context.Context, id int, newAPIKey string) error
	Destroy(ctx context.Context, id int) error
	SetLock(ctx context.Context, id int, locked bool) error
	SetLibraryLimit(ctx context.Context, id int, limit *models.LibraryLimit) error
}
