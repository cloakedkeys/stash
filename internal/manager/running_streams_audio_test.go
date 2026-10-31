package manager

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestAudioDirectStreamFileAndRanges(t *testing.T) {
	previous := instance
	instance = &Manager{ReadLockManager: fsutil.NewReadLockManager()}
	t.Cleanup(func() { instance = previous })

	// A valid PCM WAV, served through the production streaming implementation.
	wav := make([]byte, 44+1600)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(wav)-44))
	path := filepath.Join(t.TempDir(), "sample.wav")
	require.NoError(t, os.WriteFile(path, wav, 0600))

	for _, tc := range []struct {
		name, path, byteRange string
		status                int
		body                  []byte
	}{
		{"whole file", path, "", http.StatusOK, wav},
		{"byte range", path, "bytes=44-59", http.StatusPartialContent, wav[44:60]},
		{"missing source", path + ".missing", "", http.StatusNotFound, nil},
		{"no source", "", "", http.StatusNotFound, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest(http.MethodGet, "/audio/1/stream", nil).WithContext(ctx)
			if tc.byteRange != "" {
				r.Header.Set("Range", tc.byteRange)
			}
			w := httptest.NewRecorder()
			(&AudioServer{}).StreamAudioDirect(&models.Audio{Path: tc.path}, w, r)
			require.Equal(t, tc.status, w.Code)
			if tc.body != nil {
				require.Equal(t, tc.body, w.Body.Bytes())
				require.Contains(t, w.Header().Get("Content-Disposition"), "sample.wav")
				require.Equal(t, "bytes", w.Header().Get("Accept-Ranges"))
			}
			if tc.byteRange != "" {
				require.Equal(t, "bytes 44-59/1644", w.Header().Get("Content-Range"))
			}
		})
	}
}
