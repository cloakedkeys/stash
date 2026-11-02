package image

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestGetThumbnailAVIF(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	encoders, err := exec.Command(ffmpegPath, "-encoders").Output()
	require.NoError(t, err)
	if !bytes.Contains(encoders, []byte("libaom-av1")) {
		t.Skip("this ffmpeg has no libaom-av1 encoder for the AVIF fixture")
	}
	muxers, err := exec.Command(ffmpegPath, "-muxers").Output()
	require.NoError(t, err)
	if !bytes.Contains(muxers, []byte("avif")) {
		t.Skip("this ffmpeg has no AVIF muxer")
	}

	for _, tc := range []struct {
		name  string
		alpha uint8
		merge bool
	}{
		{"transparent", 0, true},
		{"semitransparent", 128, true},
		{"opaque alpha stream", 255, true},
		{"no alpha stream", 255, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
			for y := 0; y < 16; y++ {
				for x := 0; x < 32; x++ {
					a := tc.alpha
					if x < 8 {
						a = 255
					}
					img.SetNRGBA(x, y, color.NRGBA{B: 255, A: a})
				}
			}
			var input bytes.Buffer
			require.NoError(t, png.Encode(&input, thumbnailAlphaImage{img}))
			source := filepath.Join(t.TempDir(), "alpha.avif")
			args := []string{"-hide_banner", "-v", "error", "-i", "-"}
			if tc.merge {
				args = append(args, "-filter_complex",
					"[0:v]format=yuva420p,split[a][b];[a]format=yuv420p[m];[b]alphaextract,format=gray[c]",
					"-map", "[m]", "-map", "[c]")
			} else {
				args = append(args, "-vf", "format=yuv420p")
			}
			args = append(args, "-frames:v", "1", "-c:v", "libaom-av1", "-still-picture", "1",
				"-cpu-used", "8", "-crf", "0", "-b:v", "0", "-threads", "1", source)
			cmd := exec.Command(ffmpegPath, args...)
			cmd.Stdin = &input
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)

			f := &models.ImageFile{BaseFile: &models.BaseFile{Path: source}, Format: "avif", Width: 32, Height: 16}
			for _, backend := range []string{"ffmpeg", "vips"} {
				t.Run(backend, func(t *testing.T) {
					encoder := ThumbnailEncoder{FFMpeg: ffmpeg.NewEncoder(ffmpegPath), FFProbe: ffmpeg.NewFFProbe(ffprobePath)}
					if backend == "vips" {
						vipsPath, err := exec.LookPath("vips")
						if err != nil {
							t.Skip("vips is not installed")
						}
						// Some vips builds have no HEIF/AVIF loader.
						output, err := exec.Command(vipsPath, "thumbnail", source, ".png", "16").CombinedOutput()
						if err != nil && strings.Contains(string(output), "not a known file format") {
							t.Skip("this vips has no AVIF loader")
						}
						require.NoError(t, err, "%s", output)
						vips := vipsEncoder(vipsPath)
						encoder.vips = &vips
					}

					data, err := encoder.GetThumbnail(f, 16)
					require.NoError(t, err)
					decoded, _, err := image.Decode(bytes.NewReader(data))
					require.NoError(t, err)
					require.Equal(t, image.Rect(0, 0, 16, 8), decoded.Bounds())
					require.Equal(t, uint32(65535), thumbnailAlphaAt(decoded, 0, 4))
					require.InDelta(t, uint32(tc.alpha)*257, thumbnailAlphaAt(decoded, 15, 4), 257)
					pixel := color.NRGBAModel.Convert(decoded.At(0, 4)).(color.NRGBA)
					require.Greater(t, int(pixel.B), 240, "colour stream stays blue")
					require.Less(t, int(pixel.R), 16)
					if tc.alpha == 255 {
						require.Equal(t, "image/jpeg", http.DetectContentType(data))
					} else {
						require.Contains(t, []string{"image/webp", "image/png"}, http.DetectContentType(data))
					}
				})
			}
		})
	}
}
