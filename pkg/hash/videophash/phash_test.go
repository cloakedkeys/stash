package videophash

import (
	"fmt"
	"image"
	"image/color"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disintegration/imaging"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/models"
)

func TestValidHash(t *testing.T) {
	for _, hash := range []uint64{
		0x0000000000000000,
		0x8000000000000000,
		0xa000000000800080,
		0x8080808080808080,
		0x870707030787fefc,
		0x82070707078ffff8,
		0x8055557555575575,
		0x805555555d755d55,
		0x87070707037ef8fc,
		0x8707070303fefcdc,
		0xcdcdcdc9c1233332,
		0xf8fcfcfcf8e00100,
		0xf8fcfcfcfc700000,
	} {
		t.Run(fmt.Sprintf("%016x", hash), func(t *testing.T) {
			if validHash(hash) {
				t.Fatal("accepted known bad hash")
			}
			for bit := 0; bit < 64; bit++ {
				var mask uint64
				for distance := 1; distance <= 3; distance++ {
					mask |= uint64(1) << ((bit + distance - 1) % 64)
					if validHash(hash ^ mask) {
						t.Errorf("accepted bad hash with %d bits changed: %016x", distance, hash^mask)
					}
				}
			}
		})
	}

	for _, hash := range []uint64{0xa00000000080008f, 0xaa22dd22f72ad52a, 0xb1c39e786ab452cd} {
		if !validHash(hash) {
			t.Errorf("rejected valid hash: %016x", hash)
		}
	}
}

func TestGenerateValidation(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for generation tests")
	}
	encoder := ffmpeg.NewEncoder(ffmpegPath)
	for _, tt := range []struct {
		name     string
		source   string
		duration float64
		wantErr  string
	}{
		{"black video", "color=c=black:s=160x160:r=10", 2, "invalid phash"},
		{"white video", "color=c=white:s=160x160:r=10", 2, "invalid phash"},
		{"zero duration", "testsrc2=s=160x160:r=10", 0, "duration"},
		{"negative duration", "testsrc2=s=160x160:r=10", -1, "duration"},
		{"valid video", "testsrc2=s=160x160:r=10", 2, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "video.mkv")
			cmd := exec.Command(ffmpegPath, "-v", "error", "-f", "lavfi", "-i", tt.source, "-t", "2", "-c:v", "ffv1", path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("creating video: %v: %s", err, output)
			}
			file := &models.VideoFile{BaseFile: &models.BaseFile{Path: path}, Duration: tt.duration}
			hash, err := Generate(encoder, file)
			if tt.wantErr == "" {
				if err != nil || hash == nil {
					t.Fatalf("Generate() = %v, %v; want a hash", hash, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) || hash != nil {
				if hash != nil {
					t.Logf("returned phash: %016x", *hash)
				}
				t.Fatalf("Generate() = %v, %v; want nil hash and error containing %q", hash, err, tt.wantErr)
			}
		})
	}
}

func TestSpriteColumns(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		want     int
	}{
		{"one second clip", 1, 2},
		{"45 seconds", 45, 2},
		{"just over 45 seconds", 45.1, 3},
		{"90 seconds", 90, 3},
		{"just over 90 seconds", 90.1, 4},
		{"150 seconds", 150, 4},
		{"just over 150 seconds", 150.1, 5},
		{"feature length", 3600, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spriteColumns(tt.duration); got != tt.want {
				t.Errorf("spriteColumns(%v) = %d, want %d", tt.duration, got, tt.want)
			}
		})
	}
}

func TestCombineImages(t *testing.T) {
	const frameSize = 16

	for _, columns := range []int{2, 3, 4, 5} {
		chunkCount := columns * columns

		// each frame is a solid colour, keyed off its index, so that the
		// position of every frame in the montage can be verified
		images := make([]image.Image, chunkCount)
		for i := range images {
			images[i] = imaging.New(frameSize, frameSize, color.NRGBA{R: uint8(i + 1), A: 255})
		}

		montage := combineImages(images, columns)

		want := frameSize * columns
		if size := montage.Bounds().Size(); size.X != want || size.Y != want {
			t.Errorf("columns %d: montage size = %dx%d, want %dx%d", columns, size.X, size.Y, want, want)
		}

		for i := range images {
			x := frameSize * (i % columns)
			y := frameSize * (i / columns)

			r, _, _, _ := montage.At(x, y).RGBA()
			if got := uint8(r >> 8); got != uint8(i+1) {
				t.Errorf("columns %d: frame %d at (%d,%d) = %d, want %d", columns, i, x, y, got, uint8(i+1))
			}
		}
	}
}
