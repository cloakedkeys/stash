package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

func TestMigrateHashPartKeys(t *testing.T) {
	for _, algo := range []models.HashAlgorithm{models.HashAlgorithmOshash, models.HashAlgorithmMd5} {
		t.Run(algo.String(), func(t *testing.T) {
			p := paths.NewPaths(t.TempDir(), "")
			previous := instance
			instance = &Manager{Paths: &p}
			t.Cleanup(func() { instance = previous })
			start, end := 1.234, 9.876
			s := &models.Scene{OSHash: "0123456789abcdef", Checksum: "fedcba9876543210fedcba9876543210", StartTime: &start, EndTime: &end}
			oldKey, newKey := s.OSHash+"_1234-9876", s.Checksum+"_1234-9876"
			if algo == models.HashAlgorithmOshash {
				oldKey, newKey = newKey, oldKey
			}
			for _, path := range []string{p.Scene.SpriteImage(oldKey).Path, p.SceneMarkers.VideoPreview(oldKey, 30).Path} {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, os.WriteFile(path, []byte("generated"), 0600))
			}
			(&MigrateHashTask{Scene: s, fileNamingAlgorithm: algo}).Start()
			require.NoFileExists(t, p.Scene.SpriteImage(oldKey).Path)
			require.FileExists(t, p.Scene.SpriteImage(newKey).Path)
			require.FileExists(t, p.SceneMarkers.VideoPreview(newKey, 30).Path)
		})
	}
}
