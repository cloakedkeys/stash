package manager

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
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

func TestPartDirectStreamNeverServesFile(t *testing.T) {
	original := instance
	t.Cleanup(func() { instance = original })
	config.InitializeEmpty()
	p := paths.NewPaths(t.TempDir(), "")
	instance = &Manager{Paths: &p, ReadLockManager: fsutil.NewReadLockManager()}
	file := filepath.Join(t.TempDir(), "source.mp4")
	require.NoError(t, os.WriteFile(file, []byte("raw whole file"), 0600))
	start, end := 3.25, 8.75
	for _, bounds := range []struct{ start, end *float64 }{{&start, &end}, {&start, nil}, {nil, &end}, {nil, nil}} {
		s := &models.Scene{Path: file, StartTime: bounds.start, EndTime: bounds.end}
		s.Files.SetPrimary(&models.VideoFile{BaseFile: &models.BaseFile{Path: file}, Duration: 12})
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method, "/scene/1/stream/source.mp4", nil)
			r.Header.Set("Range", "bytes=0-3")
			(&SceneServer{}).StreamSceneDirect(s, w, r)
			if bounds.start != nil || bounds.end != nil {
				require.Equal(t, 503, w.Code)
				require.NotContains(t, w.Body.String(), "raw ")
			} else {
				require.Equal(t, 206, w.Code)
			}
		}
	}
}

func TestPartStreamListOmitsDirect(t *testing.T) {
	original := instance
	t.Cleanup(func() { instance = original })
	config.InitializeEmpty()
	p := paths.NewPaths(t.TempDir(), "")
	instance = &Manager{Paths: &p}
	start := 3.25
	s := &models.Scene{Checksum: "hash", OSHash: "hash"}
	s.Files.SetPrimary(&models.VideoFile{BaseFile: &models.BaseFile{Path: "source.mp4"}, Format: string(ffmpeg.Mp4), VideoCodec: "h264", AudioCodec: "aac", Duration: 12})
	u, err := url.Parse("http://localhost/scene/1/stream?apikey=test")
	require.NoError(t, err)
	whole, err := GetSceneStreamPaths(s, u, models.StreamingResolutionEnumOriginal)
	require.NoError(t, err)
	require.Equal(t, "direct", *whole[0].StreamType)
	s.StartTime = &start
	part, err := GetSceneStreamPaths(s, u, models.StreamingResolutionEnumOriginal)
	require.NoError(t, err)
	require.Equal(t, whole[1:], part)
}

func TestPartDirectStreamHTTP(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg required")
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe required")
	}
	original := instance
	t.Cleanup(func() { instance = original })
	cfg := config.InitializeEmpty()
	p := paths.NewPaths(t.TempDir(), "")
	locks := fsutil.NewReadLockManager()
	sm := ffmpeg.NewStreamManager(t.TempDir(), ffmpeg.NewEncoder(ff), ffmpeg.NewFFProbe(probe), cfg, locks)
	t.Cleanup(sm.Shutdown)
	instance = &Manager{Config: cfg, Paths: &p, StreamManager: sm, ReadLockManager: locks}
	file := filepath.Join(t.TempDir(), "source.mp4")
	out, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=20:duration=12", "-c:v", "libx264", "-g", "100", file).CombinedOutput()
	require.NoError(t, err, string(out))
	start, end := 3.25, 8.75
	s := &models.Scene{Path: file, StartTime: &start, EndTime: &end}
	s.Files.SetPrimary(&models.VideoFile{BaseFile: &models.BaseFile{Path: file}, Duration: 12, Width: 64, Height: 64, VideoCodec: "h264"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { (&SceneServer{}).StreamSceneDirect(s, w, r) }))
	defer server.Close()
	req, err := http.NewRequestWithContext(context.Background(), "GET", server.URL+"?start=1.25", nil)
	require.NoError(t, err)
	req.Header.Set("Range", "bytes=0-3")
	response, err := server.Client().Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	require.Equal(t, ffmpeg.MimeMp4Video, response.Header.Get("Content-Type"))
	require.Empty(t, response.Header.Get("Content-Range"))
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	clip := filepath.Join(t.TempDir(), "part.mp4")
	require.NoError(t, os.WriteFile(clip, data, 0600))
	out, err = exec.Command(probe, "-v", "error", "-show_entries", "format=duration", "-of", "json", clip).CombinedOutput()
	require.NoError(t, err, string(out))
	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	require.NoError(t, json.Unmarshal(out, &result))
	length, err := strconv.ParseFloat(result.Format.Duration, 64)
	require.NoError(t, err)
	require.InDelta(t, 4.25, length, 0.15)
	for _, seek := range []string{"-1", "5.5", "NaN", "Inf"} {
		response, err := server.Client().Get(server.URL + "?start=" + seek)
		require.NoError(t, err)
		require.Equal(t, 400, response.StatusCode)
		response.Body.Close()
	}
}
