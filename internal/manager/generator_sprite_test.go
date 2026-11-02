package manager

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/scene/generate"
)

func TestSpriteImageSize(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required to measure generated sprite tiles")
	}
	encoder := ffmpeg.NewEncoder(ffmpegPath)

	for _, tt := range []struct {
		name          string
		width, height int
		sar           string
		ratio         float64
		want          image.Point
	}{
		{"portrait", 320, 640, "1", 1, image.Pt(80, 160)},
		{"landscape", 640, 320, "1", 1, image.Pt(160, 80)},
		{"square", 320, 320, "1", 1, image.Pt(160, 160)},
		{"display portrait", 640, 320, "1/4", 0.25, image.Pt(80, 160)},
		{"display landscape", 320, 640, "4", 4, image.Pt(160, 80)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.mkv")
			args := ffmpeg.Args{
				"-v", "error", "-y", "-f", "lavfi",
				"-i", fmt.Sprintf("color=c=red:s=%dx%d:r=4", tt.width, tt.height),
				"-frames:v", "4", "-vf", "setsar=" + tt.sar,
				"-c:v", "ffv1", input,
			}
			if err := encoder.Generate(context.Background(), args); err != nil {
				t.Fatal(err)
			}

			for _, slow := range []bool{false, true} {
				t.Run(fmt.Sprintf("slow=%t", slow), func(t *testing.T) {
					output := filepath.Join(dir, fmt.Sprintf("sprite-%t.jpg", slow))
					g := SpriteGenerator{
						Info: &generatorInfo{
							ChunkCount: 4,
							VideoFile: ffmpeg.VideoFile{
								Path:                input,
								Width:               tt.width,
								Height:              tt.height,
								FrameCount:          4,
								VideoStreamDuration: 1,
							},
						},
						ImageOutputPath: output,
						Config:          SpriteGeneratorConfig{SpriteSize: 160},
						SlowSeek:        slow,
						g: &generate.Generator{
							Encoder:           encoder,
							LockManager:       fsutil.NewReadLockManager(),
							SampleAspectRatio: tt.ratio,
						},
					}
					if err := g.generateSpriteImage(); err != nil {
						t.Fatal(err)
					}
					f, err := os.Open(output)
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()
					config, _, err := image.DecodeConfig(f)
					if err != nil {
						t.Fatal(err)
					}
					// Four thumbnails form a 2x2 sheet; measure each tile.
					got := image.Pt(config.Width/2, config.Height/2)
					if got != tt.want {
						t.Fatalf("sprite tile = %v, want %v", got, tt.want)
					}
				})
			}
		})
	}
}
