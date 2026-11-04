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

const (
	checksumTestImageID = 1
	checksumTestFileID  = 100
	checksumTestOld     = "0123456789abcdef0123456789abcdef"
	checksumTestNew     = "fedcba9876543210fedcba9876543210"
)

// imageFiles writes the thumbnail and clip preview of checksum in both layouts.
func imageFiles(t *testing.T, root, checksum string) []string {
	t.Helper()
	newDir := filepath.Join(root, "image", checksum[0:2], checksum[2:4])
	oldDir := filepath.Join(root, "thumbnails", checksum[0:2], checksum[2:4])
	files := []string{
		filepath.Join(newDir, checksum+"_thumb_640.jpg"),
		filepath.Join(newDir, checksum+"_preview_640.webm"),
		filepath.Join(oldDir, checksum+"_640.jpg"),
		filepath.Join(oldDir, checksum+"_640.webm"),
	}
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))
	}
	return files
}

// rescanChangedFile runs the scan handler for a file whose MD5 changed from
// checksumTestOld to checksumTestNew. stillUsing are the images that a file
// with checksumTestOld still belongs to.
func rescanChangedFile(t *testing.T, p *paths.Paths, stillUsing []*models.Image) {
	t.Helper()
	file := func(hash string) *models.BaseFile {
		return &models.BaseFile{
			ID:           models.FileID(checksumTestFileID),
			Path:         "/images/clip.mp4",
			Fingerprints: models.Fingerprints{{Type: models.FingerprintTypeMD5, Fingerprint: hash}},
		}
	}
	oldFile, newFile := file(checksumTestOld), file(checksumTestNew)

	db := mocks.NewDatabase()
	image := &models.Image{
		ID:         checksumTestImageID,
		Files:      models.NewRelatedFiles([]models.File{newFile}),
		GalleryIDs: models.NewRelatedIDs([]int{}),
	}
	db.Image.On("FindByFileID", mock.Anything, models.FileID(checksumTestFileID)).Return([]*models.Image{image}, nil)
	db.Image.On("UpdatePartial", mock.Anything, checksumTestImageID, mock.Anything).Return(image, nil)
	db.Image.On("FindByFingerprints", mock.Anything, []models.Fingerprint{{Type: models.FingerprintTypeMD5, Fingerprint: checksumTestOld}}).Return(stillUsing, nil)

	h := &ScanHandler{
		CreatorUpdater: db.Image,
		GalleryFinder:  db.Gallery,
		ScanGenerator:  nopScanGenerator{},
		ScanConfig:     &mockScanConfig{},
		PluginCache:    &plugin.Cache{},
		Paths:          p,
	}

	db.WithTxnCtx(func(ctx context.Context) {
		require.NoError(t, h.Handle(ctx, newFile, oldFile))
	})
}

// #2043: when a changed image file gets a new checksum, the old checksum's
// generated files are all removed, the clip preview as well as the thumbnail.
func TestScanRemovesOldGeneratedFilesOnChecksumChange(t *testing.T) {
	root := t.TempDir()
	p := paths.NewPaths(root, "")
	oldFiles := imageFiles(t, root, checksumTestOld)
	newFiles := imageFiles(t, root, checksumTestNew)

	rescanChangedFile(t, &p, nil)

	for _, f := range oldFiles {
		require.NoFileExists(t, f, "a generated file of the old checksum")
	}
	for _, f := range newFiles {
		require.FileExists(t, f)
	}
}

// Files with the same content share generated files, so they're kept while
// another file still has the old checksum.
func TestScanKeepsGeneratedFilesOfSharedChecksum(t *testing.T) {
	root := t.TempDir()
	p := paths.NewPaths(root, "")
	oldFiles := imageFiles(t, root, checksumTestOld)

	rescanChangedFile(t, &p, []*models.Image{{ID: 2}})

	for _, f := range oldFiles {
		require.FileExists(t, f, "another image still uses it")
	}
}
