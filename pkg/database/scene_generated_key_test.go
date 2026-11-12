//go:build integration

package database_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stretchr/testify/require"
)

func TestSceneGeneratedKeyPrimaryDuration(t *testing.T) {
	runWithRollbackTxn(t, "open end survives scene lookup", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f := rangeVideoFileWith(ctx, t, "key-duration.mp4", 60.0005, models.Fingerprints{{Type: models.FingerprintTypeOshash, Fingerprint: "0123456789abcdef"}})
		s, err := newRangeScene(ctx, t, svc, "part", f64(10), nil, f.ID)
		require.NoError(t, err)
		got, err := db.Scene().Find(ctx, s.ID)
		require.NoError(t, err)
		require.Equal(t, "0123456789abcdef_10000-60001", scene.GeneratedKey(got, models.HashAlgorithmOshash))
		require.False(t, got.Files.PrimaryLoaded(), "key must work without another file lookup")
	})
}

func TestSceneGeneratedKeyDelete(t *testing.T) {
	for _, same := range []bool{false, true} {
		name := "different range"
		if same {
			name = "same range"
		}
		runWithRollbackTxn(t, name, func(t *testing.T, ctx context.Context) {
			svc := rangeService()
			f := rangeVideoFileWith(ctx, t, "key-delete.mp4", 60, models.Fingerprints{{Type: models.FingerprintTypeOshash, Fingerprint: "0123456789abcdef"}})
			target, err := newRangeScene(ctx, t, svc, "target", f64(10), f64(20), f.ID)
			require.NoError(t, err)
			start := 30.0
			if same {
				start = 10
			}
			end := start + 10
			other, err := newRangeScene(ctx, t, svc, "sibling", &start, &end, f.ID)
			require.NoError(t, err)
			for _, sc := range []*models.Scene{target, other} {
				marker := &models.SceneMarker{SceneID: sc.ID, Title: "marker", Seconds: 5, PrimaryTagID: tagIDs[tagIdxWithScene]}
				require.NoError(t, db.SceneMarker().Create(ctx, marker))
			}
			p := paths.NewPaths(t.TempDir(), "")
			ownMarker := p.SceneMarkers.VideoPreview("0123456789abcdef_10000-20000", 5).Path
			otherMarker := p.SceneMarkers.VideoPreview("0123456789abcdef_30000-40000", 5).Path
			own := p.Scene.SpriteImage("0123456789abcdef_10000-20000").Path
			sibling := p.Scene.SpriteImage("0123456789abcdef_30000-40000").Path
			for _, path := range []string{own, sibling, ownMarker, otherMarker} {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, os.WriteFile(path, []byte("generated"), 0600))
			}
			deleter := &scene.FileDeleter{Deleter: file.NewDeleter(), Paths: &p, FileNamingAlgo: models.HashAlgorithmOshash}
			require.NoError(t, svc.Destroy(ctx, target, deleter, true, true, false))
			deleter.Commit()
			if same {
				require.FileExists(t, own)
				require.FileExists(t, ownMarker)
			} else {
				require.NoFileExists(t, own)
				require.NoFileExists(t, ownMarker)
			}
			require.FileExists(t, sibling)
			require.FileExists(t, otherMarker)
			files, err := db.File().Find(ctx, f.ID)
			require.NoError(t, err)
			require.Len(t, files, 1, "shared source survives")
		})
	}
	runWithRollbackTxn(t, "last scene removes its key", func(t *testing.T, ctx context.Context) {
		svc := rangeService()
		f := rangeVideoFileWith(ctx, t, "last-key.mp4", 60, models.Fingerprints{{Type: models.FingerprintTypeOshash, Fingerprint: "0123456789abcdef"}})
		target, err := newRangeScene(ctx, t, svc, "last", f64(10), f64(20), f.ID)
		require.NoError(t, err)
		p := paths.NewPaths(t.TempDir(), "")
		own := p.Scene.SpriteImage("0123456789abcdef_10000-20000").Path
		require.NoError(t, os.MkdirAll(filepath.Dir(own), 0755))
		require.NoError(t, os.WriteFile(own, []byte("generated"), 0600))
		deleter := &scene.FileDeleter{Deleter: file.NewDeleter(), Paths: &p, FileNamingAlgo: models.HashAlgorithmOshash}
		require.NoError(t, svc.Destroy(ctx, target, deleter, true, false, false))
		deleter.Commit()
		require.NoFileExists(t, own)
	})
}
