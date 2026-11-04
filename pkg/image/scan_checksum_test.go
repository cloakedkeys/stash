package image

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type nopScanGenerator struct{}

func (nopScanGenerator) Generate(context.Context, *models.Image, models.File) error { return nil }

// #2043: when a changed image file gets a new checksum, the old checksum's
// generated files are all removed, the clip preview as well as the thumbnail.
func TestScanRemovesOldGeneratedFilesOnChecksumChange(t *testing.T) {
	const (
		imageID = 1
		fileID  = 100
		oldHash = "0123456789abcdef0123456789abcdef"
		newHash = "fedcba9876543210fedcba9876543210"
	)
	p := paths.NewPaths(t.TempDir(), "")

	var oldFiles, newFiles []string
	for _, f := range generatedFiles(&p, oldHash) {
		oldFiles = append(oldFiles, f.Both()...)
	}
	for _, f := range generatedFiles(&p, newHash) {
		newFiles = append(newFiles, f.Both()...)
	}
	for _, f := range append(oldFiles, newFiles...) {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	}

	file := func(hash string) *models.BaseFile {
		return &models.BaseFile{
			ID:           models.FileID(fileID),
			Path:         "/images/clip.mp4",
			Fingerprints: models.Fingerprints{{Type: models.FingerprintTypeMD5, Fingerprint: hash}},
		}
	}
	oldFile, newFile := file(oldHash), file(newHash)

	db := mocks.NewDatabase()
	image := &models.Image{
		ID:         imageID,
		Files:      models.NewRelatedFiles([]models.File{newFile}),
		GalleryIDs: models.NewRelatedIDs([]int{}),
	}
	db.Image.On("FindByFileID", mock.Anything, models.FileID(fileID)).Return([]*models.Image{image}, nil)
	db.Image.On("UpdatePartial", mock.Anything, imageID, mock.Anything).Return(image, nil)

	h := &ScanHandler{
		CreatorUpdater: db.Image,
		GalleryFinder:  db.Gallery,
		ScanGenerator:  nopScanGenerator{},
		ScanConfig:     &mockScanConfig{},
		PluginCache:    &plugin.Cache{},
		Paths:          &p,
	}

	db.WithTxnCtx(func(ctx context.Context) {
		require.NoError(t, h.Handle(ctx, newFile, oldFile))
	})

	for _, f := range oldFiles {
		require.NoFileExists(t, f, "a generated file of the old checksum")
	}
	for _, f := range newFiles {
		require.FileExists(t, f)
	}
}
