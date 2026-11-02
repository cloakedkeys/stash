package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file is converted to UTF-8 only while it's within the limit, judged by
// what is actually read, not the size saved at the last scan.
func TestServeStoryTextLimit(t *testing.T) {
	latin1 := []byte("caf\xe9 au lait") // 12 bytes, é in Latin-1
	path := filepath.Join(t.TempDir(), "story.txt")
	require.NoError(t, os.WriteFile(path, latin1, 0o644))

	serve := func(scannedSize, limit int64) string {
		f := &models.BaseFile{Path: path, Basename: "story.txt", Size: scannedSize}
		w := httptest.NewRecorder()
		require.NoError(t, serveStoryTextLimit(f, w, httptest.NewRequest("GET", "/story/1/text", nil), limit))
		return w.Body.String()
	}

	assert.Equal(t, "café au lait", serve(12, 12), "within the limit")
	assert.Equal(t, string(latin1), serve(12, 11), "over the limit by its scanned size")
	// scanned when it was small, and has grown since
	assert.Equal(t, string(latin1), serve(4, 11), "over the limit, though its scanned size isn't")
}
