package api

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/sliceutil/stringslice"
	"github.com/stashapp/stash/pkg/user"
)

func (r *mutationResolver) UserCreate(ctx context.Context, input UserCreateInput) (user *models.User, err error) {
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		notes := ""
		if input.Notes != nil {
			notes = *input.Notes
		}

		limit, err := libraryLimitFromInput(input.LibraryLimit)
		if err != nil {
			return err
		}

		err = r.userService.CreateUser(ctx, models.User{
			Username:     input.Name,
			Roles:        models.Roles(input.Roles),
			Notes:        notes,
			LibraryLimit: limit,
		}, input.Password)
		if err != nil {
			return fmt.Errorf("error creating user: %w", err)
		}

		user, err = r.userService.GetUser(ctx, input.Name)
		if err != nil {
			return fmt.Errorf("error getting user after creation: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return user, nil
}

func (r *mutationResolver) UserUpdate(ctx context.Context, input UserUpdateInput) (user *models.User, err error) {
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		notes := ""
		if input.Notes != nil {
			notes = *input.Notes
		}

		err := r.userService.UpdateUser(ctx, input.ExistingName, models.User{
			Username: input.Name,
			Roles:    models.Roles(input.Roles),
			Notes:    notes,
		})
		if err != nil {
			return fmt.Errorf("error updating user: %w", err)
		}

		user, err = r.userService.GetUser(ctx, input.Name)
		if err != nil {
			return fmt.Errorf("error getting user after update: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return user, nil
}

func libraryLimitFromInput(input *LibraryLimitInput) (*models.LibraryLimit, error) {
	if input == nil {
		return nil, nil
	}

	folders, err := stringslice.StringSliceToIntSlice(input.FolderIds)
	if err != nil {
		return nil, fmt.Errorf("converting folder ids: %w", err)
	}

	tags, err := stringslice.StringSliceToIntSlice(input.TagIds)
	if err != nil {
		return nil, fmt.Errorf("converting tag ids: %w", err)
	}

	studios, err := stringslice.StringSliceToIntSlice(input.StudioIds)
	if err != nil {
		return nil, fmt.Errorf("converting studio ids: %w", err)
	}

	return &models.LibraryLimit{FolderIDs: folders, TagIDs: tags, StudioIDs: studios}, nil
}

func (r *mutationResolver) UserSetLibraryLimit(ctx context.Context, input UserSetLibraryLimitInput) (user *models.User, err error) {
	limit, err := libraryLimitFromInput(input.LibraryLimit)
	if err != nil {
		return nil, err
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		if err := r.userService.SetLibraryLimit(ctx, input.Name, limit); err != nil {
			return fmt.Errorf("error setting library limit: %w", err)
		}

		user, err = r.userService.GetUser(ctx, input.Name)
		if err != nil {
			return fmt.Errorf("error getting user after update: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return user, nil
}

func (r *mutationResolver) UserDestroy(ctx context.Context, input UserDestroyInput) (bool, error) {
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		err := r.userService.DeleteUser(ctx, input.Name)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) ChangePassword(ctx context.Context, input UserChangePasswordInput) (bool, error) {
	// get current user
	u := session.GetCurrentUser(ctx)
	if u == nil {
		return false, fmt.Errorf("no current user in context")
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		return r.userService.ChangePassword(ctx, u.Username, input.ExistingPassword, input.NewPassword)
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) ChangeUserPassword(ctx context.Context, input ChangeUserPasswordInput) (bool, error) {
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		return r.userService.ChangeUserPassword(ctx, input.Name, input.NewPassword)
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) GenerateAPIKey(ctx context.Context, input GenerateAPIKeyInput) (string, error) {
	u := session.GetCurrentUser(ctx)
	if u == nil {
		return "", fmt.Errorf("no current user in context")
	}

	return r.generateUserAPIKey(ctx, u, input)
}

func (r *mutationResolver) GenerateUserAPIKey(ctx context.Context, username string, input GenerateAPIKeyInput) (string, error) {
	var u *models.User
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		var err error
		u, err = r.userService.GetUser(ctx, username)
		return err
	}); err != nil {
		return "", fmt.Errorf("error retrieving user: %w", err)
	}

	if u == nil {
		return "", fmt.Errorf("user %q: %w", username, user.ErrUserNotExist)
	}

	return r.generateUserAPIKey(ctx, u, input)
}

func (r *mutationResolver) LockUser(ctx context.Context, name string) (bool, error) {
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		return r.userService.LockUser(ctx, name)
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) UnlockUser(ctx context.Context, name string) (bool, error) {
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		return r.userService.UnlockUser(ctx, name)
	}); err != nil {
		return false, err
	}

	return true, nil
}
func (r *mutationResolver) generateUserAPIKey(ctx context.Context, u *models.User, input GenerateAPIKeyInput) (string, error) {
	var newAPIKey string
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		var err error

		if input.Clear != nil && *input.Clear {
			return r.userService.ClearAPIKey(ctx, u.Username)
		}

		newAPIKey, err = r.userService.GenerateAPIKey(ctx, u.Username)
		return err
	}); err != nil {
		return "", err
	}

	return newAPIKey, nil
}
