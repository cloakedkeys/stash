package scene

import (
	"context"
	"errors"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Each transition drops the old files only on commit, preserving equal-key siblings.
func TestChangedGeneratedKeyLifecycle(t *testing.T) {
	ptr := func(f float64) *float64 { return &f }
	for _, tc := range []struct {
		name          string
		before, after *float64
	}{
		{"range change", ptr(10), ptr(20)}, {"part to whole", ptr(10), nil}, {"whole to part", nil, ptr(10)},
	} {
		for _, sibling := range []bool{false, true} {
			for _, rollback := range []bool{false, true} {
				t.Run(tc.name+"/sibling="+boolName(sibling)+"/rollback="+boolName(rollback), func(t *testing.T) {
					p := paths.NewPaths(t.TempDir(), "")
					old := &models.Scene{ID: 1, OSHash: "0123456789abcdef", StartTime: tc.before, PrimaryFileDuration: 60}
					updated := *old
					updated.StartTime = tc.after
					key := GeneratedKey(old, models.HashAlgorithmOshash)
					path := p.Scene.SpriteImage(key).Path
					seedStaleFile(t, path)
					db := mocks.NewDatabase()
					scenes := []*models.Scene{&updated}
					if sibling {
						other := *old
						other.ID = 2
						scenes = append(scenes, &other)
					}
					db.Scene.On("FindByFingerprints", mock.Anything, mock.Anything).Return(scenes, nil).Maybe()
					err := txn.WithTxn(context.Background(), db, func(ctx context.Context) error {
						require.NoError(t, InvalidateChangedGeneratedKey(ctx, db.Scene, &p, models.HashAlgorithmOshash, old, &updated))
						require.FileExists(t, path)
						if rollback {
							return errors.New("rollback")
						}
						return nil
					})
					if rollback {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
					if sibling || rollback {
						require.FileExists(t, path)
					} else {
						require.NoFileExists(t, path)
					}
				})
			}
		}
	}
}

func boolName(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func TestImporterInvalidatesReplacedGeneratedKey(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name                               string
		oldStart, oldEnd, newStart, newEnd *float64
	}{
		{"bounds change", ptr(10), ptr(20), ptr(10), ptr(30)},
		{"part to whole", ptr(10), ptr(20), nil, nil},
		{"whole to part", nil, nil, ptr(10), ptr(20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := &models.Scene{ID: 1, OSHash: "0123456789abcdef", StartTime: tc.oldStart, EndTime: tc.oldEnd}
			updated := *old
			updated.StartTime, updated.EndTime = tc.newStart, tc.newEnd
			p := paths.NewPaths(t.TempDir(), "")
			oldPath := p.Scene.VideoPreview(GeneratedKey(old, models.HashAlgorithmOshash)).Path
			seedStaleFile(t, oldPath)
			db := mocks.NewDatabase()
			db.Scene.On("Find", mock.Anything, 1).Return(old, nil).Once()
			db.Scene.On("Update", mock.Anything, mock.Anything).Return(nil).Once()
			db.Scene.On("Find", mock.Anything, 1).Return(&updated, nil).Once()
			db.Scene.On("FindByFingerprints", mock.Anything, mock.Anything).Return([]*models.Scene{&updated}, nil).Once()
			i := Importer{ReaderWriter: db.Scene, Paths: &p, FileNamingAlgorithm: models.HashAlgorithmOshash, scene: updated}
			withCommittedTxn(t, db, func(ctx context.Context) error { return i.Update(ctx, 1) })
			require.NoFileExists(t, oldPath)
			db.Scene.AssertExpectations(t)
		})
	}
}
