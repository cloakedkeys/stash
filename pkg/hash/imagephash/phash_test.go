package imagephash

import (
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/corona10/goimagehash"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/models"
	_ "golang.org/x/image/bmp"
)

func TestGenerateAnimatedGIF(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 32, 32), palette)
	second := image.NewPaletted(first.Bounds(), palette)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if x < y {
				first.SetColorIndex(x, y, 1)
			}
			if x+y < 32 {
				second.SetColorIndex(x, y, 1)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "animated.gif")
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = gif.EncodeAll(out, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{10, 10}})
	closeErr := out.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	want, err := goimagehash.PerceptionHash(first)
	if err != nil {
		t.Fatal(err)
	}

	// GIF clips are stored as VideoFile, but their first frame is still
	// decodable without ffmpeg. Static ImageFile decoding must also work.
	for _, f := range []models.File{
		&models.ImageFile{BaseFile: &models.BaseFile{Path: path}},
		&models.VideoFile{BaseFile: &models.BaseFile{Path: path}},
	} {
		got, err := Generate(nil, f)
		if err != nil {
			t.Fatalf("Generate(%T): %v", f, err)
		}
		if got == nil || *got != want.GetHash() {
			t.Errorf("Generate(%T) = %v, want first-frame hash %016x", f, got, want.GetHash())
		}
	}
}

func TestGenerateVideoImage(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for video image phash tests")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	cmd := exec.Command(ffmpegPath, "-v", "error", "-f", "lavfi", "-i", "testsrc2=s=64x64:r=2", "-frames:v", "2", "-c:v", "mpeg4", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating video: %v: %s", err, output)
	}

	// Compare with an independently extracted first frame, rather than a
	// video montage hash, so clips stay comparable with ordinary images.
	cmd = exec.Command(ffmpegPath, "-v", "error", "-i", path, "-frames:v", "1", "-c:v", "bmp", "-f", "image2pipe", "-")
	frame, err := cmd.Output()
	if err != nil {
		t.Fatalf("extracting first frame: %v", err)
	}
	framePath := filepath.Join(t.TempDir(), "first.bmp")
	if err := os.WriteFile(framePath, frame, 0600); err != nil {
		t.Fatal(err)
	}
	want, err := Generate(nil, &models.ImageFile{BaseFile: &models.BaseFile{Path: framePath}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Generate(ffmpeg.NewEncoder(ffmpegPath), &models.VideoFile{BaseFile: &models.BaseFile{Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || want == nil || *got != *want {
		t.Fatalf("video hash = %v, want first-frame hash %v", got, want)
	}
}
