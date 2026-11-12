package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

type partStreamConfig struct{}

func (partStreamConfig) GetMaxStreamingTranscodeSize() models.StreamingResolutionEnum {
	return models.StreamingResolutionEnumOriginal
}
func (partStreamConfig) GetLiveTranscodeInputArgs() []string    { return nil }
func (partStreamConfig) GetLiveTranscodeOutputArgs() []string   { return nil }
func (partStreamConfig) GetTranscodeHardwareAcceleration() bool { return false }
func rangeNumber(v float64) *float64                            { return &v }
func rangeManager() *StreamManager {
	return &StreamManager{config: partStreamConfig{}, encoder: &FFMpeg{}, context: context.Background()}
}
func rangeFile() *models.VideoFile {
	return &models.VideoFile{BaseFile: &models.BaseFile{Path: "source.mp4"}, Duration: 12, Width: 64, Height: 64, FrameRate: 20, VideoCodec: "h264", AudioCodec: "aac"}
}
func rangeArg(args Args, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestPartLiveArguments(t *testing.T) {
	for _, format := range []StreamFormat{StreamTypeMP4, StreamTypeWEBM, StreamTypeMKV} {
		for _, seek := range []float64{0, 1.25} {
			t.Run(fmt.Sprintf("%s/%g", format.MimeType, seek), func(t *testing.T) {
				o := TranscodeOptions{StreamType: format, VideoFile: rangeFile(), StartTime: seek, Range: StreamRange{Start: rangeNumber(3.25), End: rangeNumber(8.75)}}
				args := o.makeStreamArgs(rangeManager())
				require.Equal(t, fmt.Sprint(3.25+seek), rangeArg(args, "-ss"))
				require.Equal(t, fmt.Sprint(5.5-seek), rangeArg(args, "-t"))
				require.NotEqual(t, "copy", rangeArg(args, "-c:v"), "a part must decode through its first keyframe")
				require.NotContains(t, args, "-noaccurate_seek")
				require.Contains(t, args, "-sn")
			})
		}
	}
}

func TestPartSegmentArguments(t *testing.T) {
	for _, typ := range []*StreamType{StreamTypeHLS, StreamTypeHLSCopy, StreamTypeDASHVideo, StreamTypeDASHAudio} {
		for _, n := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/%d", typ.Name, n), func(t *testing.T) {
				s := &runningStream{streamType: typ, vf: rangeFile(), playbackRange: StreamRange{Start: rangeNumber(3.25), End: rangeNumber(8.75)}, outputDir: "out"}
				args := s.makeStreamArgs(rangeManager(), n)
				require.Equal(t, fmt.Sprint(3.25+float64(n*segmentLength)), rangeArg(args, "-ss"))
				require.Equal(t, fmt.Sprint(5.5-float64(n*segmentLength)), rangeArg(args, "-t"))
				require.Equal(t, fmt.Sprint(n*segmentLength), rangeArg(args, "-output_ts_offset"))
				require.NotContains(t, args, "-copyts")
			})
		}
	}
}

// This snapshot is captured from develop before changing the stream arguments.
func TestWholeFileStreamArguments(t *testing.T) {
	got := map[string]Args{}
	for _, format := range []StreamFormat{StreamTypeMP4, StreamTypeWEBM, StreamTypeMKV} {
		for _, seek := range []float64{0, 1.25} {
			o := TranscodeOptions{StreamType: format, VideoFile: rangeFile(), StartTime: seek}
			got[fmt.Sprintf("%s/%g", format.MimeType, seek)] = o.makeStreamArgs(rangeManager())
		}
	}
	for _, typ := range []*StreamType{StreamTypeHLS, StreamTypeHLSCopy, StreamTypeDASHVideo, StreamTypeDASHAudio} {
		for _, n := range []int{0, 2} {
			s := &runningStream{streamType: typ, vf: rangeFile(), outputDir: "out"}
			a := s.makeStreamArgs(rangeManager(), n)
			for i := range a {
				a[i] = filepath.ToSlash(a[i])
			}
			got[fmt.Sprintf("%s/%d", typ.Name, n)] = a
		}
	}
	b, err := os.ReadFile("testdata/whole_stream_args.json")
	require.NoError(t, err)
	var want map[string]Args
	require.NoError(t, json.Unmarshal(b, &want))
	require.Equal(t, want, got)
}

func partSource(t *testing.T) (*StreamManager, *models.VideoFile) {
	t.Helper()
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is required for media verification")
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is required for media verification")
	}
	file := filepath.Join(t.TempDir(), "source.mp4")
	out, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=20:duration=12", "-f", "lavfi", "-i", "sine=frequency=440:duration=12", "-c:v", "libx264", "-g", "100", "-c:a", "aac", file).CombinedOutput()
	require.NoError(t, err, string(out))
	sm := rangeManager()
	sm.encoder = NewEncoder(ff)
	sm.ffprobe = NewFFProbe(probe)
	sm.cacheDir = t.TempDir()
	vf := rangeFile()
	vf.Path = file
	return sm, vf
}

func TestPartManifestsAndSegmentBounds(t *testing.T) {
	sm, vf := partSource(t)
	for _, tc := range []struct {
		name   string
		r      StreamRange
		length float64
	}{
		{"bounded", StreamRange{Start: rangeNumber(3.25), End: rangeNumber(8.75)}, 5.5},
		{"start only", StreamRange{Start: rangeNumber(3.25)}, 8.75},
		{"end only", StreamRange{End: rangeNumber(5.5)}, 5.5},
		{"whole", StreamRange{}, 12},
	} {
		for _, typ := range []*StreamType{StreamTypeHLS, StreamTypeDASHVideo} {
			t.Run(tc.name+"/"+typ.Name, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "http://localhost/scene/1/stream", nil)
				sm.ServeManifest(w, req, typ, vf, "", tc.r)
				require.Equal(t, 200, w.Code, w.Body.String())
				if typ == StreamTypeHLS {
					require.Equal(t, int((tc.length+1.999999)/2), strings.Count(w.Body.String(), "#EXTINF:"))
					sum := 0.0
					for _, line := range strings.Split(w.Body.String(), "\n") {
						if strings.HasPrefix(line, "#EXTINF:") {
							v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ","), 64)
							require.NoError(t, err)
							sum += v
						}
					}
					require.InDelta(t, tc.length, sum, 0.001)
				} else {
					require.Contains(t, w.Body.String(), fmt.Sprintf("mediaPresentationDuration=\"PT%gS\"", tc.length))
				}
				// Requests past the scene's last segment must be refused before ffmpeg starts.
				w = httptest.NewRecorder()
				sm.ServeSegment(w, req, StreamOptions{StreamType: typ, VideoFile: vf, Hash: "hash", Segment: fmt.Sprint(int((tc.length + 1.999999) / 2)), Range: tc.r})
				require.Equal(t, 400, w.Code)
			})
		}
	}
}

func TestPartCopyCompatibleCodecsStillEncode(t *testing.T) {
	for _, codec := range []string{"h264", "vp8", "vp9"} {
		for _, format := range []StreamFormat{StreamTypeMP4, StreamTypeWEBM, StreamTypeMKV} {
			vf := rangeFile()
			vf.VideoCodec = codec
			o := TranscodeOptions{VideoFile: vf, StreamType: format, Range: StreamRange{Start: rangeNumber(3.25)}}
			require.NotEqual(t, VideoCodecCopy, o.FileGetCodec(rangeManager(), 0), "%s/%s", codec, format.MimeType)
		}
	}
}

func TestPartStreamCacheIsolation(t *testing.T) {
	opts := StreamOptions{StreamType: StreamTypeHLS, Hash: "hash"}
	require.Equal(t, "hash_hls", opts.cacheDir(0))
	require.Equal(t, "hash_hls_720", opts.cacheDir(720))
	keys := map[string]bool{opts.cacheDir(0): true}
	for _, r := range []StreamRange{
		{End: rangeNumber(5.5)}, {Start: rangeNumber(3.25), End: rangeNumber(8.75)},
		{Start: rangeNumber(3.25001), End: rangeNumber(8.75)}, {Start: rangeNumber(3.25)},
	} {
		opts.Range = r
		k := opts.cacheDir(0)
		require.False(t, keys[k], "cache collision: %s", k)
		keys[k] = true
	}
}

func TestPartMediaTimestamps(t *testing.T) {
	sm, vf := partSource(t)
	r := StreamRange{Start: rangeNumber(3.25), End: rangeNumber(8.75)}
	check := func(t *testing.T, file string, start, end float64) {
		t.Helper()
		out, err := exec.Command(sm.ffprobe.Path(), "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,duration_time", "-of", "json", file).CombinedOutput()
		require.NoError(t, err, string(out))
		var probe struct {
			Packets []struct {
				PTS      string `json:"pts_time"`
				Duration string `json:"duration_time"`
			} `json:"packets"`
		}
		require.NoError(t, json.Unmarshal(out, &probe))
		require.NotEmpty(t, probe.Packets)
		first, last := 1e9, -1e9
		for _, p := range probe.Packets {
			v, err := strconv.ParseFloat(p.PTS, 64)
			require.NoError(t, err)
			d, _ := strconv.ParseFloat(p.Duration, 64)
			first = min(first, v)
			last = max(last, v+d)
		}
		require.InDelta(t, start, first, 0.16)
		require.InDelta(t, end, last, 0.16)
	}
	for _, format := range []StreamFormat{StreamTypeMP4, StreamTypeWEBM, StreamTypeMKV} {
		t.Run(format.MimeType, func(t *testing.T) {
			for _, seek := range []float64{0, 1.25} {
				o := TranscodeOptions{VideoFile: vf, StreamType: format, StartTime: seek, Range: r}
				args := o.makeStreamArgs(sm)
				file := filepath.Join(t.TempDir(), "clip")
				args[len(args)-1] = file
				out, err := sm.encoder.Command(context.Background(), args).CombinedOutput()
				require.NoError(t, err, string(out))
				check(t, file, 0, 5.5-seek)
			}
		})
	}
	for _, typ := range []*StreamType{StreamTypeHLS, StreamTypeDASHVideo} {
		for _, n := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s/%d", typ.Name, n), func(t *testing.T) {
				dir := t.TempDir()
				s := &runningStream{streamType: typ, vf: vf, playbackRange: r, outputDir: dir}
				out, err := sm.encoder.Command(context.Background(), s.makeStreamArgs(sm, n)).CombinedOutput()
				require.NoError(t, err, string(out))
				if typ == StreamTypeHLS {
					for i := n; i < 3; i++ {
						check(t, filepath.Join(dir, fmt.Sprintf(".%d.ts", i)), float64(i*2)+1.4, min(float64((i+1)*2), 5.5)+1.4)
					}
				} else {
					init := "init_v.webm"
					if n > 0 {
						init = ".init_v.webm"
					}
					data, err := os.ReadFile(filepath.Join(dir, init))
					require.NoError(t, err)
					for i := n; i < 3; i++ {
						chunk, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf(".%d_v.webm", i)))
						require.NoError(t, err)
						data = append(data, chunk...)
					}
					file := filepath.Join(dir, "combined.webm")
					require.NoError(t, os.WriteFile(file, data, 0600))
					check(t, file, float64(n*2), 5.5)
				}
			})
		}
	}
}
