package file

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type recordingHandler struct {
	oldMD5, newMD5 string
}

func (h *recordingHandler) Handle(ctx context.Context, f models.File, oldFile models.File) error {
	h.newMD5 = f.Base().Fingerprints.GetString(models.FingerprintTypeMD5)
	h.oldMD5 = oldFile.Base().Fingerprints.GetString(models.FingerprintTypeMD5)
	return nil
}

// #2043: the handlers of a changed file get its old fingerprints as well as
// its new ones, so that they can tell its checksum changed and remove the
// generated files of the old one.
func TestScannerChangedFileHandlersGetOldFingerprints(t *testing.T) {
	ctx := context.Background()
	db := mocks.NewDatabase()
	modTime := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	existing := &models.ImageFile{
		BaseFile: &models.BaseFile{
			ID:           1,
			Path:         filepath.Join("Test", "clip.mp4"),
			Basename:     "clip.mp4",
			Size:         100,
			DirEntry:     models.DirEntry{ModTime: modTime},
			Fingerprints: []models.Fingerprint{{Type: models.FingerprintTypeMD5, Fingerprint: "old-md5"}},
		},
	}
	h := &recordingHandler{}
	scanner := Scanner{
		Repository: Repository{
			TxnManager: db.Repository().TxnManager,
			File:       db.File,
		},
		FingerprintCalculator: scanTestFingerprintCalculator{fingerprints: []models.Fingerprint{
			{Type: models.FingerprintTypeMD5, Fingerprint: "new-md5"},
		}},
		FileHandlers: []Handler{h},
	}
	scanned := ScannedFile{
		BaseFile: &models.BaseFile{
			Path:     existing.Path,
			Basename: existing.Basename,
			Size:     200,
			DirEntry: models.DirEntry{ModTime: modTime.Add(time.Hour)},
		},
		FS: scanTestFS{caseSensitive: true},
	}
	db.File.On("FindByPath", mock.Anything, scanned.Path, true).Return(existing, nil).Once()
	db.File.On("Update", mock.Anything, mock.Anything).Return(nil).Once()

	_, err := scanner.ScanFile(ctx, scanned)
	require.NoError(t, err)

	require.Equal(t, "new-md5", h.newMD5)
	require.Equal(t, "old-md5", h.oldMD5)
}
