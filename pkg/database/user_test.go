//go:build integration
// +build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserUpdateKeepsCredentials(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.User()
		created := time.Now().Add(-time.Hour).Truncate(time.Second)

		u := models.User{
			Username:  "update-keeps-credentials",
			Roles:     models.Roles{models.RoleEnumRead},
			CreatedAt: created,
			UpdatedAt: created,
		}
		require.NoError(t, qb.Create(ctx, &u, "password-hash"))
		require.NoError(t, qb.SetUserAPIKey(ctx, u.ID, "api-key-hash"))

		updatedAt := time.Now().Truncate(time.Second)
		require.NoError(t, qb.Update(ctx, &models.User{
			ID:        u.ID,
			Username:  "update-keeps-credentials-renamed",
			Notes:     "edited",
			Roles:     models.Roles{models.RoleEnumRead, models.RoleEnumModify},
			UpdatedAt: updatedAt,
		}))

		got, err := qb.FindByUsername(ctx, "update-keeps-credentials-renamed")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, "edited", got.Notes)
		assert.Len(t, got.Roles, 2)
		assert.Equal(t, "api-key-hash", got.ApiKeyHash)
		assert.True(t, got.CreatedAt.Equal(created), "created time changed to %v", got.CreatedAt)
		assert.True(t, got.UpdatedAt.Equal(updatedAt), "updated time is %v", got.UpdatedAt)

		hash, err := qb.GetPasswordHash(ctx, u.ID)
		require.NoError(t, err)
		assert.Equal(t, "password-hash", hash)

		return nil
	}))
}

func TestUserSetLockUpdatesTimestamp(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		qb := db.User()
		created := time.Now().Add(-time.Hour)

		u := models.User{
			Username:  "lock-updates-timestamp",
			Roles:     models.Roles{models.RoleEnumRead},
			CreatedAt: created,
			UpdatedAt: created,
		}
		require.NoError(t, qb.Create(ctx, &u, "password-hash"))

		require.NoError(t, qb.SetLock(ctx, u.ID, true))

		got, err := qb.FindByUsername(ctx, u.Username)
		require.NoError(t, err)
		assert.True(t, got.Locked)
		assert.True(t, got.UpdatedAt.After(created.Add(time.Minute)), "updated time is %v", got.UpdatedAt)

		return nil
	}))
}
