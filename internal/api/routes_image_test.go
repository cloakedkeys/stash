package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestThumbnailContentType(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		mime string
	}{
		{"JPEG", []byte{0xff, 0xd8, 0xff, 0xe0}, "image/jpeg"},
		{"WebP", []byte("RIFF\x0c\x00\x00\x00WEBPVP8L"), "image/webp"},
		{"PNG fallback", []byte("\x89PNG\r\n\x1a\n"), "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Keep the existing cache filename, including for regenerated files.
			path := filepath.Join(t.TempDir(), "checksum_320.jpg")
			require.NoError(t, os.WriteFile(path, tc.data, 0600))
			modTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, mt := range []*time.Time{nil, &modTime} {
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodGet, "/image/1/thumbnail", nil)
				serveImageThumbnailFile(w, r, path, mt)
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, tc.mime, w.Header().Get("Content-Type"))
				require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
				require.Equal(t, tc.data, w.Body.Bytes())
				if mt != nil {
					require.Equal(t, mt.Format(http.TimeFormat), w.Header().Get("Last-Modified"))
				}
			}
			w := httptest.NewRecorder()
			utils.ServeStaticContent(w, httptest.NewRequest(http.MethodGet, "/image/1/thumbnail", nil), tc.data)
			require.Equal(t, tc.mime, w.Header().Get("Content-Type"), "in-memory thumbnail")
			stored, err := os.ReadFile(path)
			require.NoError(t, err)
			require.True(t, bytes.Equal(tc.data, stored), "serving does not regenerate existing thumbnails")
		})
	}
}
