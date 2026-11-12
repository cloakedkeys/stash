package scene

import (
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

// Whole-file names stay compatible under both naming algorithms.
func TestGeneratedKey(t *testing.T) {
	start, end := 1.234, 9.876
	for _, r := range []struct {
		name       string
		start, end *float64
	}{
		{"whole", nil, nil}, {"bounded", &start, &end},
		{"start only", &start, nil}, {"end only", nil, &end},
	} {
		for _, algo := range []models.HashAlgorithm{models.HashAlgorithmOshash, models.HashAlgorithmMd5} {
			t.Run(r.name+"/"+string(algo), func(t *testing.T) {
				s := &models.Scene{OSHash: "0123456789abcdef", Checksum: "0123456789abcdef0123456789abcdef", StartTime: r.start, EndTime: r.end}
				want := s.OSHash
				if algo == models.HashAlgorithmMd5 {
					want = s.Checksum
				}
				if r.name != "whole" {
					suffix := "_1234-9876"
					if r.name == "start only" {
						suffix = "_1234-end"
					}
					if r.name == "end only" {
						suffix = "_0-9876"
					}
					want += suffix
				}
				require.Equal(t, want, GeneratedKey(s, algo))
				// Keys depend on the file, not the scene's database identity.
				s.ID = 123
				require.Equal(t, want, GeneratedKey(s, algo))
				require.Empty(t, GeneratedKey(&models.Scene{}, algo))
			})
		}
	}
}

// Pin both layouts through the generated key.
func TestGeneratedKeyPaths(t *testing.T) {
	start, end := 1.234, 9.876
	for _, part := range []bool{false, true} {
		for _, algo := range []models.HashAlgorithm{models.HashAlgorithmOshash, models.HashAlgorithmMd5} {
			s := &models.Scene{OSHash: "0123456789abcdef", Checksum: "0123456789abcdef0123456789abcdef"}
			if part {
				s.StartTime, s.EndTime = &start, &end
			}
			hash := s.OSHash
			if algo == models.HashAlgorithmMd5 {
				hash = s.Checksum
			}
			if part {
				hash += "_1234-9876"
			}
			p := paths.NewPaths(t.TempDir(), "")
			key := GeneratedKey(s, algo)
			for _, tc := range []struct {
				file         paths.GeneratedFile
				kind, legacy string
			}{
				{p.Scene.VideoPreview(key), "preview.mp4", filepath.Join("screenshots", hash+".mp4")},
				{p.Scene.WebpPreview(key), "preview.webp", filepath.Join("screenshots", hash+".webp")},
				{p.Scene.Transcode(key), "transcode.mp4", filepath.Join("transcodes", hash+".mp4")},
				{p.Scene.SpriteImage(key), "sprite.jpg", filepath.Join("vtt", hash+"_sprite.jpg")},
				{p.Scene.SpriteVtt(key), "thumbs.vtt", filepath.Join("vtt", hash+"_thumbs.vtt")},
				{p.Scene.Heatmap(key), "heatmap.png", filepath.Join("interactive_heatmaps", hash+".png")},
				{p.SceneMarkers.VideoPreview(key, 30), "marker_30.mp4", filepath.Join("markers", hash, "30.mp4")},
				{p.SceneMarkers.WebpPreview(key, 30), "marker_30.webp", filepath.Join("markers", hash, "30.webp")},
				{p.SceneMarkers.Screenshot(key, 30), "marker_30.jpg", filepath.Join("markers", hash, "30.jpg")},
			} {
				require.Equal(t, filepath.Join(p.Generated.Video, "01", "23", hash+"_"+tc.kind), tc.file.Path)
				require.Equal(t, filepath.Join(p.Generated.Screenshots, "..", tc.legacy), tc.file.Legacy)
			}
			require.Equal(t, filepath.Join(p.Generated.Screenshots, hash+".jpg"), p.Scene.GetLegacyScreenshotPath(key))
		}
	}
}

func TestGeneratedKeyOpenEndAndRounding(t *testing.T) {
	start, end := 1.2345, 9.8765
	s := &models.Scene{OSHash: "0123456789abcdef", StartTime: &start, EndTime: &end}
	require.Equal(t, "0123456789abcdef_1235-9877", GeneratedKey(s, models.HashAlgorithmOshash))
	s.EndTime = nil
	s.PrimaryFileDuration = 42.0005
	require.Equal(t, "0123456789abcdef_1235-42001", GeneratedKey(s, models.HashAlgorithmOshash))
	s.PrimaryFileDuration = 0
	require.Equal(t, "0123456789abcdef_1235-end", GeneratedKey(s, models.HashAlgorithmOshash))
	s.Files.SetPrimary(&models.VideoFile{Duration: 50})
	require.Equal(t, "0123456789abcdef_1235-50000", GeneratedKey(s, models.HashAlgorithmOshash))
}
