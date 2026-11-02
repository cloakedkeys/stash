package manager

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/internal/static"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

type screenshotTxn struct{}

func (screenshotTxn) Begin(ctx context.Context, _ bool) (context.Context, error) {
	return ctx, nil
}
func (screenshotTxn) Commit(context.Context) error {
	return nil
}
func (screenshotTxn) Rollback(context.Context) error {
	return nil
}
func (screenshotTxn) IsLocked(error) bool {
	return false
}

type screenshotCover []byte

func (c screenshotCover) GetCover(context.Context, int) ([]byte, error) { return c, nil }

func TestSceneScreenshotThumbnail(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		want          image.Rectangle
		alpha         uint8
		jpeg          bool
	}{
		{"large PNG", 2560, 1440, image.Rect(0, 0, 640, 360), 255, false},
		{"large JPEG", 2560, 1440, image.Rect(0, 0, 640, 360), 255, true},
		{"portrait", 800, 1600, image.Rect(0, 0, 320, 640), 255, false},
		{"small cover is not enlarged", 160, 90, image.Rect(0, 0, 160, 90), 255, false},
		{"transparent cover", 1280, 720, image.Rect(0, 0, 640, 360), 128, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, tc.width, tc.height))
			for y := 0; y < tc.height; y++ {
				for x := 0; x < tc.width; x++ {
					img.SetNRGBA(x, y, color.NRGBA{R: 100, G: 150, B: 200, A: tc.alpha})
				}
			}
			var input bytes.Buffer
			if tc.jpeg {
				require.NoError(t, jpeg.Encode(&input, img, nil))
			} else {
				require.NoError(t, png.Encode(&input, img))
			}
			cover := append([]byte(nil), input.Bytes()...)
			server := SceneServer{TxnManager: screenshotTxn{}, SceneCoverGetter: screenshotCover(cover)}
			for _, thumbnail := range []bool{false, true} {
				url := "/scene/1/screenshot?t=123"
				if thumbnail {
					url += "&thumbnail=true"
				}
				w := httptest.NewRecorder()
				server.ServeScreenshot(&models.Scene{ID: 1}, w, httptest.NewRequest(http.MethodGet, url, nil))
				require.Equal(t, http.StatusOK, w.Code)
				if !thumbnail {
					require.Equal(t, cover, w.Body.Bytes(), "original cover must stay byte-for-byte intact")
					continue
				}
				if tc.width <= 640 && tc.height <= 640 {
					require.Equal(t, cover, w.Body.Bytes(), "small cover must stay byte-for-byte intact")
				}
				decoded, _, err := image.Decode(bytes.NewReader(w.Body.Bytes()))
				require.NoError(t, err)
				require.Equal(t, tc.want, decoded.Bounds())
				_, _, _, alpha := decoded.At(0, 0).RGBA()
				require.Equal(t, uint32(tc.alpha)*257, alpha)
				require.Equal(t, input.Bytes(), cover, "stored data must not change")
			}
		})
	}
}

func TestSceneScreenshotThumbnailFallbacks(t *testing.T) {
	for _, cover := range [][]byte{
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		[]byte("invalid image"),
		nil,
	} {
		server := SceneServer{TxnManager: screenshotTxn{}, SceneCoverGetter: screenshotCover(cover)}
		w := httptest.NewRecorder()
		server.ServeScreenshot(&models.Scene{ID: 1}, w, httptest.NewRequest(http.MethodGet, "/scene/1/screenshot?thumbnail=true", nil))
		if cover == nil {
			cover = static.ReadAll(static.DefaultSceneImage)
		}
		require.Equal(t, cover, w.Body.Bytes())
	}
}

func TestSceneScreenshotLegacyThumbnail(t *testing.T) {
	previous := instance
	t.Cleanup(func() { instance = previous })
	config.InitializeEmpty()
	p := paths.NewPaths(t.TempDir(), "")
	instance = &Manager{Paths: &p}
	scene := &models.Scene{ID: 1, Path: "scene.mp4", Checksum: "cover", OSHash: "cover"}
	coverPath := p.Scene.GetLegacyScreenshotPath(scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm()))
	require.NoError(t, os.MkdirAll(filepath.Dir(coverPath), 0700))
	var cover bytes.Buffer
	require.NoError(t, jpeg.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 1280, 720)), nil))
	require.NoError(t, os.WriteFile(coverPath, cover.Bytes(), 0600))
	server := SceneServer{TxnManager: screenshotTxn{}, SceneCoverGetter: screenshotCover(nil)}
	w := httptest.NewRecorder()
	server.ServeScreenshot(scene, w, httptest.NewRequest(http.MethodGet, "/scene/1/screenshot?thumbnail=true", nil))
	decoded, _, err := image.Decode(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 640, 360), decoded.Bounds())
}
