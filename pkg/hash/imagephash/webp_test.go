package imagephash

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/corona10/goimagehash"
	"github.com/stashapp/stash/pkg/models"
)

// The tiny VP8L fixtures encode a 32x32 green triangle on black, with
// alpha 255 (first.webp) or 128 (alpha.webp). They use literal pixels,
// two green Huffman symbols and one symbol for each other channel.
func TestGenerateAnimatedWebP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture string
		alpha   uint8
		offset  int
		flags   byte
	}{
		{"opaque", "first.webp", 255, 0, 0},
		{"alpha, offset on the canvas", "alpha.webp", 128, 4, 0},
		{"alpha, not blended", "alpha.webp", 128, 4, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			still, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := image.Decode(bytes.NewReader(still))
			if err != nil {
				t.Fatal(err)
			}
			first := image.NewNRGBA(image.Rect(0, 0, 32, 32))
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					g := uint8(0)
					if x < y {
						g = 255
					}
					pixel := color.NRGBA{G: g, A: tc.alpha}
					first.SetNRGBA(x, y, pixel)
					if got := color.NRGBAModel.Convert(decoded.At(x, y)); got != pixel {
						t.Fatalf("fixture pixel (%d, %d) = %v, want %v", x, y, got, pixel)
					}
				}
			}
			// the first frame on its own, wherever it sits on the canvas and
			// however it would be blended, as for GIF
			canvasSize := 32 + 2*tc.offset
			wantImage := first
			want, err := goimagehash.PerceptionHash(wantImage)
			if err != nil {
				t.Fatal(err)
			}
			animated := animatedWebPTestData(still, canvasSize, tc.offset, tc.flags)
			// This is the failure in the old loader: Go recognizes WebP but
			// cannot find image chunks inside the animation's ANMF chunks.
			if _, _, err := image.Decode(bytes.NewReader(animated)); err == nil {
				t.Fatal("standard decoder unexpectedly decoded animated WebP")
			}
			path := filepath.Join(t.TempDir(), "animated.webp")
			if err := os.WriteFile(path, animated, 0600); err != nil {
				t.Fatal(err)
			}
			var zipData bytes.Buffer
			zw := zip.NewWriter(&zipData)
			w, err := zw.Create("animated.webp")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(animated); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			zipPath := filepath.Join(t.TempDir(), "images.zip")
			if err := os.WriteFile(zipPath, zipData.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			zf := &models.BaseFile{ID: 1, Path: zipPath, Size: int64(zipData.Len())}
			for _, f := range []models.File{
				&models.ImageFile{BaseFile: &models.BaseFile{Path: path}},
				&models.VideoFile{BaseFile: &models.BaseFile{Path: path}},
				&models.ImageFile{BaseFile: &models.BaseFile{
					Path:     filepath.Join(zipPath, "animated.webp"),
					DirEntry: models.DirEntry{ZipFileID: &zf.ID, ZipFile: zf},
				}},
			} {
				// A nil encoder proves this path does not depend on ffmpeg.
				got, err := Generate(nil, f)
				if err != nil {
					t.Fatalf("Generate(%T): %v", f, err)
				}
				if got == nil || *got != want.GetHash() {
					t.Fatalf("Generate(%T) = %v, want first-frame hash %016x", f, got, want.GetHash())
				}
				loaded, err := loadImage(nil, f)
				if err != nil {
					t.Fatal(err)
				}
				if loaded.Bounds() != wantImage.Bounds() || !bytes.Equal(loaded.(*image.NRGBA).Pix, wantImage.Pix) {
					t.Fatal("decoded first frame differs from expected pixels")
				}
			}
			// Still WebP must continue to use the ordinary decoder.
			if err := os.WriteFile(path, still, 0600); err != nil {
				t.Fatal(err)
			}
			wantStill, err := goimagehash.PerceptionHash(first)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Generate(nil, &models.ImageFile{BaseFile: &models.BaseFile{Path: path}})
			if err != nil || got == nil || *got != wantStill.GetHash() {
				t.Fatalf("static WebP hash = %v, %v", got, err)
			}
		})
	}
}

func webPTestChunk(id string, data []byte) []byte {
	header := make([]byte, 8)
	copy(header, id)
	binary.LittleEndian.PutUint32(header[4:], uint32(len(data)))
	header = append(header, data...)
	if len(data)%2 != 0 {
		header = append(header, 0)
	}
	return header
}

func animatedWebPTestData(still []byte, canvasSize, offset int, flags byte) []byte {
	extended := []byte{0x12, 0, 0, 0, byte(canvasSize - 1), 0, 0, byte(canvasSize - 1), 0, 0}
	data := webPTestChunk("VP8X", extended)
	// Include an odd-sized unknown chunk to exercise RIFF padding.
	data = append(data, webPTestChunk("JUNK", []byte{1, 2, 3})...)
	data = append(data, webPTestChunk("ANIM", []byte{0, 0, 255, 255, 0, 0})...)
	frame := make([]byte, 16)
	frame[0], frame[3] = byte(offset/2), byte(offset/2)
	frame[6], frame[9], frame[12], frame[15] = 31, 31, 100, flags
	frame = append(frame, still[12:]...)
	data = append(data, webPTestChunk("ANMF", frame)...)
	data = append(data, webPTestChunk("ANMF", frame)...)
	header := make([]byte, 12)
	copy(header, "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(len(data)+4))
	copy(header[8:], "WEBP")
	return append(header, data...)
}

func TestLoadAnimatedWebPMalformed(t *testing.T) {
	still, err := os.ReadFile(filepath.Join("testdata", "first.webp"))
	if err != nil {
		t.Fatal(err)
	}
	valid := animatedWebPTestData(still, 32, 0, 0)
	for _, end := range []int{0, 12, 29, 40, 60, 80, 100} {
		if _, err := loadAnimatedWebP(bytes.NewReader(valid[:end])); err == nil {
			t.Errorf("truncated at %d: expected error", end)
		}
	}
	outside := animatedWebPTestData(still, 32, 4, 0)
	if _, err := loadAnimatedWebP(bytes.NewReader(outside)); err == nil {
		t.Error("frame outside canvas: expected error")
	}
}
