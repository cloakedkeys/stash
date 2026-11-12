package scene

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// #2043: a copy of a file in another scene shares its generated files, so
// they're kept when the file changes while the copy still has the old hash.
// The changed scene's generated cover is still cleared.
func TestHandle_ContentChangedAtSamePath_CopyInAnotherScene(t *testing.T) {
	for _, algo := range []models.HashAlgorithm{models.HashAlgorithmOshash, models.HashAlgorithmMd5} {
		t.Run(algo.String(), func(t *testing.T) { testCopyInAnotherScene(t, algo) })
	}
}

func testCopyInAnotherScene(t *testing.T, algo models.HashAlgorithm) {
	fpType := models.FingerprintTypeOshash
	if algo == models.HashAlgorithmMd5 {
		fpType = models.FingerprintTypeMD5
	}
	const (
		testSceneID  = 1
		otherSceneID = 2
		testFileID   = 100
	)

	file := func(hash string) *models.VideoFile {
		return &models.VideoFile{BaseFile: &models.BaseFile{
			ID:           models.FileID(testFileID),
			Path:         "test.mp4",
			Fingerprints: models.Fingerprints{{Type: fpType, Fingerprint: hash}},
		}}
	}
	oldFile, newFile := file(oldHash), file(newHash)

	primaryFileID := oldFile.ID
	scene := &models.Scene{
		ID:             testSceneID,
		CoverGenerated: true,
		PrimaryFileID:  &primaryFileID,
		Files:          models.NewRelatedVideoFiles([]*models.VideoFile{oldFile}),
	}

	tmpDir := t.TempDir()
	p := paths.NewPaths(tmpDir, filepath.Join(tmpDir, "blobs"))
	sharedSprite := p.Scene.GetSpriteImageFilePath(oldHash)
	seedStaleFile(t, sharedSprite)

	db := mocks.NewDatabase()
	db.File.On("GetCaptions", mock.Anything, mock.Anything).Return(nil, nil)
	db.Scene.On("FindByFileID", mock.Anything, models.FileID(testFileID)).Return([]*models.Scene{scene}, nil)
	db.Scene.On("GetFiles", mock.Anything, testSceneID).Return([]*models.VideoFile{oldFile}, nil)
	db.Scene.On("UpdatePartial", mock.Anything, testSceneID, mock.Anything).Return(&models.Scene{ID: testSceneID}, nil)
	db.Scene.On("UpdateCover", mock.Anything, testSceneID, []byte(nil)).Return(nil).Once()
	db.Scene.On("FindByFingerprints", mock.Anything, []models.Fingerprint{{Type: fpType, Fingerprint: oldHash}}).
		Return([]*models.Scene{{ID: otherSceneID, OSHash: oldHash, Checksum: oldHash}}, nil)
	db.Gallery.On("FindByPath", mock.Anything, mock.Anything).Return(nil, nil)

	h := newTestScanHandler(db, &p)
	h.FileNamingAlgorithm = algo

	withCommittedTxn(t, db, func(ctx context.Context) error {
		return h.Handle(ctx, newFile, oldFile)
	})

	assert.FileExists(t, sharedSprite, "the other scene still uses it")
	assert.True(t, scene.RefreshCover)
	db.Scene.AssertCalled(t, "UpdateCover", mock.Anything, testSceneID, []byte(nil))
}

func TestScanInvalidatesEachPartKey(t *testing.T) {
	for _, keepEqualRange := range []bool{false, true} {
		t.Run(boolName(keepEqualRange), func(t *testing.T) {
			const hash = "0123456789abcdef"
			makeFile := func(h string, d float64) *models.VideoFile {
				return &models.VideoFile{BaseFile: &models.BaseFile{ID: 100, Path: "test.mp4", Fingerprints: models.Fingerprints{{Type: models.FingerprintTypeOshash, Fingerprint: h}}}, Duration: d}
			}
			oldFile, newFile := makeFile(hash, 60), makeFile("fedcba9876543210", 90)
			start, end := 10.0, 20.0
			bounded := &models.Scene{ID: 1, PrimaryFileID: &oldFile.ID, OSHash: hash, StartTime: &start, EndTime: &end, Files: models.NewRelatedVideoFiles([]*models.VideoFile{newFile})}
			open := *bounded
			open.ID = 2
			open.EndTime = nil
			whole := *bounded
			whole.ID = 3
			whole.StartTime, whole.EndTime = nil, nil
			p := paths.NewPaths(t.TempDir(), "")
			boundedPath := p.Scene.SpriteImage(hash + "_10000-20000").Path
			openPath := p.Scene.SpriteImage(hash + "_10000-60000").Path
			wholePath := p.Scene.SpriteImage(hash).Path
			for _, path := range []string{boundedPath, openPath, wholePath} {
				seedStaleFile(t, path)
			}
			db := mocks.NewDatabase()
			db.File.On("GetCaptions", mock.Anything, mock.Anything).Return(nil, nil)
			db.Scene.On("FindByFileID", mock.Anything, models.FileID(100)).Return([]*models.Scene{bounded, &open, &whole}, nil)
			db.Scene.On("UpdatePartial", mock.Anything, mock.Anything, mock.Anything).Return(&models.Scene{}, nil)
			dup := &models.Scene{ID: 4, OSHash: hash}
			if keepEqualRange {
				dup.StartTime, dup.EndTime = &start, &end
			}
			db.Scene.On("FindByFingerprints", mock.Anything, mock.Anything).Return([]*models.Scene{dup}, nil)
			db.Gallery.On("FindByPath", mock.Anything, mock.Anything).Return(nil, nil)
			h := newTestScanHandler(db, &p)
			withCommittedTxn(t, db, func(ctx context.Context) error { return h.Handle(ctx, newFile, oldFile) })
			if keepEqualRange {
				assert.FileExists(t, boundedPath)
				assert.NoFileExists(t, wholePath)
			} else {
				assert.NoFileExists(t, boundedPath)
				assert.FileExists(t, wholePath)
			}
			assert.NoFileExists(t, openPath, "the old duration names the obsolete open-ended key")
		})
	}
}
