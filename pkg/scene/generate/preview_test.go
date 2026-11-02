package generate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
)

type previewTestPaths struct {
	ScenePaths
	dir string
}

func (p previewTestPaths) TempFile(pattern string) (*os.File, error) {
	return os.CreateTemp(p.dir, pattern)
}

type previewTestProbe func(string) (*ffmpeg.VideoFile, error)

func (p previewTestProbe) NewVideoFile(path string) (*ffmpeg.VideoFile, error) {
	return p(path)
}

func TestGenerateConcatFile(t *testing.T) {
	probeErr := errors.New("probe failed")
	tests := []struct {
		name     string
		segments []string
		want     string
		wantErr  string
	}{
		{
			name:     "skip empty and stream-less segments and preserve order",
			segments: []string{"empty", "video", "streamless", "empty", "audio", "video", "empty"},
			want:     "file '1.mp4'\nfile '5.mp4'\n",
		},
		{
			name:     "keep a single usable segment",
			segments: []string{"video"},
			want:     "file '0.mp4'\n",
		},
		{
			name:    "no segments",
			wantErr: "no usable segments",
		},
		{
			name:     "all segments empty",
			segments: []string{"empty", "empty"},
			wantErr:  "no usable segments",
		},
		{
			name:     "no segment has video",
			segments: []string{"streamless", "audio"},
			wantErr:  "no usable segments",
		},
		{
			name:     "missing segment is an error",
			segments: []string{"missing"},
			wantErr:  "checking preview segment",
		},
		{
			name:     "probe failure is an error",
			segments: []string{"probe error"},
			wantErr:  "probing preview segment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var files, wantProbed, probed []string
			kinds := make(map[string]string)
			for i, kind := range tt.segments {
				path := filepath.Join(dir, strconv.Itoa(i)+".mp4")
				files = append(files, path)
				kinds[path] = kind
				if kind == "missing" {
					continue
				}
				var data []byte
				if kind != "empty" {
					data = []byte("nonempty segment")
					wantProbed = append(wantProbed, path)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}

			g := Generator{
				ScenePaths: previewTestPaths{dir: dir},
				FFProbe: previewTestProbe(func(path string) (*ffmpeg.VideoFile, error) {
					probed = append(probed, path)
					video := &ffmpeg.VideoFile{}
					switch kinds[path] {
					case "video":
						video.VideoStream = &ffmpeg.FFProbeStream{}
					case "audio":
						video.AudioStream = &ffmpeg.FFProbeStream{}
					case "probe error":
						return nil, probeErr
					case "empty":
						t.Error("empty segment should not be probed")
					}
					return video, nil
				}),
			}
			path, err := g.generateConcatFile(files)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(probed, wantProbed) {
				t.Errorf("probed = %v, want %v", probed, wantProbed)
			}
			if len(tt.segments) == 1 && tt.segments[0] == "probe error" && !errors.Is(err, probeErr) {
				t.Errorf("error = %v, want wrapped probe error", err)
			}
			if tt.wantErr == "" {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != tt.want {
					t.Errorf("concat list = %q, want %q", data, tt.want)
				}
			}
			// The caller needs the temporary path for cleanup, including on failure.
			if path == "" {
				t.Error("concat path is empty")
			}
		})
	}
}
