package manager

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
)

func TestSceneStreamHeadAvailability(t *testing.T) {
	original := instance
	t.Cleanup(func() { instance = original })
	config.InitializeEmpty()
	p := paths.NewPaths(t.TempDir(), "")
	instance = &Manager{
		Paths:           &p,
		ReadLockManager: fsutil.NewReadLockManager(),
		// No encoder or probe: HEAD must only check the playable file.
	}

	for _, tt := range []struct {
		name      string
		original  bool
		transcode bool
		want      int
	}{
		{"missing file", false, false, http.StatusNotFound},
		{"existing unsupported format", true, false, http.StatusOK},
		{"generated transcode with missing original", false, true, http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &models.Scene{Path: filepath.Join(t.TempDir(), "scene.mp4"), Checksum: tt.name, OSHash: tt.name}
			if tt.original {
				if err := os.WriteFile(s.Path, []byte("unsupported codec"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tt.transcode {
				path := p.Scene.GetTranscodePath(tt.name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("generated transcode"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				(&SceneServer{}).StreamSceneDirect(s, w, r)
			}))
			defer server.Close()
			response, err := server.Client().Head(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tt.want || len(body) != 0 {
				t.Fatalf("HEAD = status %d, %d body bytes; want status %d, no body", response.StatusCode, len(body), tt.want)
			}
		})
	}
}
