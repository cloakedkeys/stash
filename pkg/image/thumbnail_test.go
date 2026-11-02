package image

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func thumbnailTestPNG(t *testing.T, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			a := alpha
			if x < 8 {
				a = 255
			}
			img.SetNRGBA(x, y, color.NRGBA{A: a})
		}
	}
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, img))
	return data.Bytes()
}

// Force an alpha channel even when every pixel is opaque.
type thumbnailAlphaImage struct {
	image.Image
}

func (thumbnailAlphaImage) Opaque() bool { return false }

func thumbnailTestOpaqueAlphaPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, thumbnailAlphaImage{img}))
	return data.Bytes()
}

func TestShouldDecodeThumbnail(t *testing.T) {
	opaque := thumbnailTestPNG(t, 255)
	config, format, err := image.DecodeConfig(bytes.NewReader(opaque))
	require.NoError(t, err)
	require.Equal(t, "png", format)
	require.Equal(t, color.RGBAModel, config.ColorModel)
	require.False(t, shouldDecodeThumbnail(opaque), "truecolour PNG without alpha skips full decode")

	alpha := thumbnailTestOpaqueAlphaPNG(t)
	config, _, err = image.DecodeConfig(bytes.NewReader(alpha))
	require.NoError(t, err)
	require.Equal(t, color.NRGBAModel, config.ColorModel)
	require.True(t, shouldDecodeThumbnail(alpha), "alpha values still need checking")

	// Change only IHDR dimensions and checksum, so checking the limit never
	// allocates a huge image. DecodeConfig does not read the pixel data.
	large := thumbnailTestPNG(t, 0)
	binary.BigEndian.PutUint32(large[16:20], 10000)
	binary.BigEndian.PutUint32(large[20:24], 10000)
	binary.BigEndian.PutUint32(large[29:33], crc32.ChecksumIEEE(large[12:29]))
	require.True(t, shouldDecodeThumbnail(large), "exactly 100 million pixels is allowed")
	binary.BigEndian.PutUint32(large[20:24], 10001)
	binary.BigEndian.PutUint32(large[29:33], crc32.ChecksumIEEE(large[12:29]))
	config, _, err = image.DecodeConfig(bytes.NewReader(large))
	require.NoError(t, err)
	require.Equal(t, color.NRGBAModel, config.ColorModel)
	require.False(t, shouldDecodeThumbnail(large), "over 100 million pixels skips full decode")
	require.False(t, shouldDecodeThumbnail([]byte("not an image")))

	paletted := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Transparent, color.Black})
	var palettePNG bytes.Buffer
	require.NoError(t, png.Encode(&palettePNG, paletted))
	require.True(t, shouldDecodeThumbnail(palettePNG.Bytes()), "paletted PNG can hold transparency")
}

func TestShouldDecodeThumbnailWebP(t *testing.T) {
	for _, alpha := range []bool{false, true} {
		// A VP8X header is enough for WebP DecodeConfig.
		data := append([]byte("RIFF\x16\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00"), make([]byte, 10)...)
		if alpha {
			data[20] = 0x10
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		require.NoError(t, err)
		if alpha {
			require.Equal(t, color.NYCbCrAModel, config.ColorModel)
		} else {
			require.Equal(t, color.YCbCrModel, config.ColorModel)
		}
		require.Equal(t, alpha, shouldDecodeThumbnail(data))
	}

	// VP8L reports NRGBA regardless of the alpha-used bit.
	for _, alphaBit := range []byte{0, 0x10} {
		data := []byte("RIFF\x12\x00\x00\x00WEBPVP8L\x05\x00\x00\x00\x2f\x00\x00\x00\x00\x00")
		data[24] = alphaBit
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		require.NoError(t, err)
		require.Equal(t, color.NRGBAModel, config.ColorModel)
		require.True(t, shouldDecodeThumbnail(data))
	}
}

func thumbnailTestFile(t *testing.T, data []byte, format string) *models.ImageFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "image")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return &models.ImageFile{BaseFile: &models.BaseFile{Path: path}, Format: format, Width: 32, Height: 16}
}

func requireThumbnailAlpha(t *testing.T, data []byte, alpha uint32) {
	t.Helper()
	decoded, _, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 16, 8), decoded.Bounds())
	require.Equal(t, alpha, thumbnailAlphaAt(decoded, 15, 4))
	require.Equal(t, uint32(65535), thumbnailAlphaAt(decoded, 0, 4), "black line stays opaque")
}

func thumbnailAlphaAt(img image.Image, x, y int) uint32 {
	return uint32(color.Alpha16Model.Convert(img.At(x, y)).(color.Alpha16).A)
}

// This exercises GetThumbnail without requiring an installed ffmpeg. Any WebP
// encoding error, including an ffmpeg build without libwebp, must preserve alpha.
func TestGetThumbnailTransparentFallback(t *testing.T) {
	encoder := ThumbnailEncoder{FFMpeg: ffmpeg.NewEncoder(filepath.Join(t.TempDir(), "missing-ffmpeg"))}
	for _, alpha := range []uint8{0, 128} {
		f := thumbnailTestFile(t, thumbnailTestPNG(t, alpha), "png")
		data, err := encoder.GetThumbnail(f, 16)
		require.NoError(t, err)
		require.Equal(t, "image/png", http.DetectContentType(data))
		requireThumbnailAlpha(t, data, uint32(alpha)*257)
	}
}

func TestGetThumbnailTransparentZipFallback(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	entry, err := zw.Create("line.png")
	require.NoError(t, err)
	_, err = entry.Write(thumbnailTestPNG(t, 0))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zipPath := filepath.Join(t.TempDir(), "images.zip")
	require.NoError(t, os.WriteFile(zipPath, archive.Bytes(), 0600))
	zipID := models.FileID(1)
	f := &models.ImageFile{
		BaseFile: &models.BaseFile{Path: filepath.Join(zipPath, "line.png"), DirEntry: models.DirEntry{
			ZipFileID: &zipID,
			ZipFile:   &models.BaseFile{Path: zipPath, Size: int64(archive.Len())},
		}},
		Format: "png",
	}
	encoder := ThumbnailEncoder{FFMpeg: ffmpeg.NewEncoder(filepath.Join(t.TempDir(), "missing-ffmpeg"))}
	data, err := encoder.GetThumbnail(f, 16)
	require.NoError(t, err)
	requireThumbnailAlpha(t, data, 0)
}

func TestGetThumbnailFFMpeg(t *testing.T) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	encoder := ThumbnailEncoder{FFMpeg: ffmpeg.NewEncoder(path)}

	for name, input := range map[string][]byte{
		"truecolour PNG without alpha stays JPEG": thumbnailTestPNG(t, 255),
		"opaque PNG with alpha stays JPEG":        thumbnailTestOpaqueAlphaPNG(t),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := encoder.GetThumbnail(thumbnailTestFile(t, input, "png"), 16)
			require.NoError(t, err)
			require.Equal(t, "image/jpeg", http.DetectContentType(data))
			expected, err := encoder.ffmpegImageThumbnail(bytes.NewBuffer(input), 16)
			require.NoError(t, err)
			require.Equal(t, expected, data, "opaque encoding is unchanged")
		})
	}

	t.Run("transparent PNG and WebP", func(t *testing.T) {
		encoders, err := exec.Command(path, "-encoders").Output()
		require.NoError(t, err)
		if !bytes.Contains(encoders, []byte("libwebp")) {
			t.Skip("this ffmpeg has no libwebp encoder; fallback is tested separately")
		}
		f := thumbnailTestFile(t, thumbnailTestPNG(t, 0), "png")
		data, err := encoder.GetThumbnail(f, 16)
		require.NoError(t, err)
		require.Equal(t, "image/webp", http.DetectContentType(data))
		requireThumbnailAlpha(t, data, 0)
		f = thumbnailTestFile(t, data, "webp")
		data, err = encoder.GetThumbnail(f, 16)
		require.NoError(t, err)
		require.Equal(t, "image/webp", http.DetectContentType(data))
		requireThumbnailAlpha(t, data, 0)
	})

	t.Run("format without Go decoder uses existing encoder", func(t *testing.T) {
		// PPM has a signature ffmpeg can detect on stdin, but no Go decoder.
		ppm := append([]byte("P6\n32 16\n255\n"), make([]byte, 32*16*3)...)
		_, _, err := image.DecodeConfig(bytes.NewReader(ppm))
		require.Error(t, err)
		require.False(t, shouldDecodeThumbnail(ppm))
		data, err := encoder.GetThumbnail(thumbnailTestFile(t, ppm, "ppm"), 16)
		require.NoError(t, err)
		require.Equal(t, "image/jpeg", http.DetectContentType(data))
		expected, err := encoder.ffmpegImageThumbnail(bytes.NewBuffer(ppm), 16)
		require.NoError(t, err)
		require.Equal(t, expected, data)
	})
}
