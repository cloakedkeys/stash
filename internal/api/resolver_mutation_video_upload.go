package api

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stashapp/stash/internal/manager"
)

func (r *mutationResolver) SceneUpload(ctx context.Context, file graphql.Upload, libraryPath string) (*VideoFile, error) {
	f, err := manager.GetInstance().UploadVideo(ctx, libraryPath, file.Filename, file.File)
	if err != nil {
		return nil, err
	}
	return &VideoFile{VideoFile: f}, nil
}
