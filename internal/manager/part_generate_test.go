package manager

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/scene/generate"
	"github.com/stretchr/testify/require"
)

func TestPartSpriteGeneration(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg required")
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe required")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "colours.mkv")
	encoder := ffmpeg.NewEncoder(ff)
	require.NoError(t, encoder.Generate(context.Background(), ffmpeg.Args{"-v", "error", "-y", "-f", "lavfi", "-i", "color=red:s=64x64:r=10:d=2", "-f", "lavfi", "-i", "color=blue:s=64x64:r=10:d=2", "-f", "lavfi", "-i", "color=green:s=64x64:r=10:d=2", "-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1:a=0[v]", "-map", "[v]", "-c:v", "ffv1", input}))
	cfg := config.InitializeEmpty()
	cfg.SetInterface(config.UseCustomSpriteInterval, true)
	cfg.SetInterface(config.MinimumSprites, 4)
	cfg.SetInterface(config.MaximumSprites, 4)
	cfg.SetInterface(config.GenerationThreads, 1)
	cfg.SetInterface(config.DefaultMarkerPreviewDuration, 20)
	previous := instance
	t.Cleanup(func() { instance = previous })
	for _, tc := range []struct {
		name       string
		start, end *float64
		duration   float64
		blue       bool
	}{
		{"whole", nil, nil, 6, false},
		{"bounded", partGenerateNumber(2), partGenerateNumber(4), 2, true},
		{"short", partGenerateNumber(2.5), partGenerateNumber(2.9), 0.4, true},
		{"open end", partGenerateNumber(2), nil, 4, false},
		{"open start", nil, partGenerateNumber(2), 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg.SetInterface(config.MinimumSprites, 4)
			cfg.SetInterface(config.MaximumSprites, 4)
			if tc.name == "short" {
				cfg.SetInterface(config.MinimumSprites, 81)
				cfg.SetInterface(config.MaximumSprites, 81)
			}
			p := paths.NewPaths(t.TempDir(), "")
			instance = &Manager{Config: cfg, Paths: &p, FFMpeg: encoder, FFProbe: ffmpeg.NewFFProbe(probe), ReadLockManager: fsutil.NewReadLockManager()}
			s := models.Scene{Path: input, OSHash: "0123456789abcdef", StartTime: tc.start, EndTime: tc.end, PrimaryFileDuration: 6}
			task := GenerateSpriteTask{Scene: s, Overwrite: true, fileNamingAlgorithm: models.HashAlgorithmOshash}
			task.Start(context.Background())
			key := scene.GeneratedKey(&s, models.HashAlgorithmOshash)
			if tc.name == "bounded" {
				g := &generate.Generator{Encoder: encoder, FFProbe: instance.FFProbe, FFMpegConfig: cfg, LockManager: instance.ReadLockManager, ScenePaths: p.Scene, MarkerPaths: p.SceneMarkers, Overwrite: true}
				preview := GeneratePreviewTask{Scene: s, VideoPreview: true, ImagePreview: true, Options: generate.PreviewOptions{Segments: 2, SegmentDuration: 1, Preset: "ultrafast"}, Overwrite: true, fileNamingAlgorithm: models.HashAlgorithmOshash, generator: g}
				preview.Start(context.Background())
				vf := &models.VideoFile{BaseFile: &models.BaseFile{Path: input}, Width: 64, Height: 64, Duration: 6}
				markers := GenerateMarkersTask{VideoPreview: true, ImagePreview: true, Screenshot: true, fileNamingAlgorithm: models.HashAlgorithmOshash, generator: g}
				require.NoError(t, markers.generateMarker(vf, &s, &models.SceneMarker{Seconds: 1.5}, ffmpeg.VideoCodecLibX264, false))
				for _, output := range []string{p.Scene.GetVideoPreviewPath(key), p.Scene.GetWebpPreviewPath(key), p.SceneMarkers.GetVideoPreviewPath(key, 1), p.SceneMarkers.GetWebpPreviewPath(key, 1), p.SceneMarkers.GetScreenshotPath(key, 1)} {
					require.FileExists(t, output)
					img, err := g.SpriteScreenshot(context.Background(), output, 0, 64, false)
					require.NoError(t, err)
					r, green, blue, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
					require.Greater(t, blue, r*2)
					require.Greater(t, blue, green*2)
				}
				for _, output := range []string{p.SceneMarkers.GetVideoPreviewPath(key, 1), p.SceneMarkers.GetWebpPreviewPath(key, 1)} {
					media, err := instance.FFProbe.NewVideoFile(output)
					require.NoError(t, err)
					// WebP containers may not report duration; MP4 must be capped.
					if media.VideoStreamDuration > 0 {
						require.LessOrEqual(t, media.VideoStreamDuration, 0.6)
					}
				}
			}

			b, err := os.ReadFile(p.Scene.GetSpriteVttFilePath(key))
			require.NoError(t, err)
			cues := strings.Split(string(b), "\n")
			require.Equal(t, "00:00:00.000", strings.Split(cues[2], " --> ")[0])
			var last string
			for _, line := range cues {
				if strings.Contains(line, " --> ") {
					last = line
				}
			}
			// Millisecond precision must include the complete scene, even for short parts.
			require.Contains(t, last, " --> "+fmt.Sprintf("00:00:%06.3f", tc.duration))
			f, err := os.Open(p.Scene.GetSpriteImageFilePath(key))
			require.NoError(t, err)
			defer f.Close()
			img, _, err := image.Decode(f)
			require.NoError(t, err)
			if tc.blue {
				grid := 2
				if tc.name == "short" {
					grid = 9
				}
				for y := 0; y < grid; y++ {
					for x := 0; x < grid; x++ {
						r, g, b, _ := img.At((2*x+1)*img.Bounds().Dx()/(grid*2), (2*y+1)*img.Bounds().Dy()/(grid*2)).RGBA()
						require.Greater(t, b, r*2)
						require.Greater(t, b, g*2)
					}
				}
			}
		})
	}
}

func partGenerateNumber(v float64) *float64 { return &v }

func TestPartStoredTranscodesSkipped(t *testing.T) {
	for _, bounds := range []models.Scene{{StartTime: partGenerateNumber(2)}, {EndTime: partGenerateNumber(4)}, {StartTime: partGenerateNumber(2), EndTime: partGenerateNumber(4)}} {
		for _, force := range []bool{false, true} {
			for _, overwrite := range []bool{false, true} {
				bounds.Path = "source.mkv"
				bounds.Files.SetPrimary(&models.VideoFile{BaseFile: &models.BaseFile{Path: bounds.Path}, Duration: 10})
				task := GenerateTranscodeTask{Scene: bounds, Force: force, Overwrite: overwrite}
				require.False(t, task.required())
				// No manager or encoder is supplied: skipping must happen before any I/O.
				task.Start(context.Background())
			}
		}
	}
}
