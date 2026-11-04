package task

import (
	"context"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

// #1139: a library started after the hash layout has no legacy folders, and
// nothing in them to clean.
func TestCleanGeneratedWithoutLegacyFolders(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	j := CleanGeneratedJob{
		Options:                  CleanGeneratedOptions{Sprites: true, Screenshots: true, Transcodes: true, Markers: true, ImageThumbnails: true},
		Paths:                    &p,
		Repository:               mocks.NewDatabase().Repository(),
		BlobsStorageType:         config.BlobStorageTypeFilesystem,
		VideoFileNamingAlgorithm: models.HashAlgorithmOshash,
	}

	for _, clean := range []func(context.Context) error{
		func(ctx context.Context) error { return j.cleanSpriteFiles(ctx, nil) },
		func(ctx context.Context) error { return j.cleanScreenshotFiles(ctx, nil) },
		func(ctx context.Context) error { return j.cleanTranscodeFiles(ctx, nil) },
		func(ctx context.Context) error { return j.cleanMarkerFiles(ctx, nil) },
		func(ctx context.Context) error { return j.cleanThumbnailFiles(ctx, nil) },
		func(ctx context.Context) error { return j.cleanVideoFiles(ctx, nil) },
	} {
		require.NoError(t, clean(context.Background()))
	}
}

func TestMigrateScreenshotsWithoutLegacyFolder(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	j := MigrateSceneScreenshotsJob{ScreenshotsPath: p.Generated.Screenshots}
	require.NoError(t, j.Execute(context.Background(), nil))
}
