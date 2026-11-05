package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/logger"
)

const hwProbeError = "No VA display found for device /dev/dri/renderD128."

// Run the test executable as a fake ffmpeg, without requiring a GPU or shell.
func TestMain(m *testing.M) {
	if mode := os.Getenv("STASH_TEST_HW_PROBE"); mode != "" {
		// "supported" accepts h264_vaapi, "supported:<encoder>" accepts that encoder
		if supported, ok := strings.CutPrefix(mode, "supported"); ok {
			encoder := "h264_vaapi"
			if name, ok := strings.CutPrefix(supported, ":"); ok {
				encoder = name
			}
			for i, arg := range os.Args {
				if arg == "-c:v" && i+1 < len(os.Args) && os.Args[i+1] == encoder {
					os.Exit(0)
				}
			}
		}
		if mode != "empty" {
			fmt.Fprintln(os.Stderr, hwProbeError)
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

type hwProbeLogger struct {
	logger.BasicLogger
	info  strings.Builder
	debug strings.Builder
}

func (l *hwProbeLogger) Info(args ...interface{}) {
	fmt.Fprintln(&l.info, args...)
}

func (l *hwProbeLogger) Infof(format string, args ...interface{}) {
	fmt.Fprintf(&l.info, format+"\n", args...)
}

func (l *hwProbeLogger) Debugf(format string, args ...interface{}) {
	fmt.Fprintf(&l.debug, format+"\n", args...)
}

func (l *hwProbeLogger) Debug(args ...interface{}) {
	fmt.Fprintln(&l.debug, args...)
}

func (l *hwProbeLogger) Tracef(string, ...interface{}) {}

func TestInitHWSupportDiagnostics(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		mode      string
		timeout   string
		wantCount int
		wantError string
	}{
		{"stderr", "failed", "10", 0, hwProbeError},
		{"empty stderr", "empty", "10", 0, "exit status 1"},
		{"timeout", "failed", "0", 0, "test timed out after 0s"},
		{"supported codec", "supported", "10", 1, hwProbeError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STASH_TEST_HW_PROBE", tc.mode)
			t.Setenv("STASH_HW_TEST_TIMEOUT", tc.timeout)
			log := &hwProbeLogger{}
			previous := logger.Logger
			logger.Logger = log
			t.Cleanup(func() { logger.Logger = previous })

			f := &FFMpeg{ffmpeg: executable}
			f.initHWSupport(context.Background())
			if got := len(f.getHWCodecSupport()); got != tc.wantCount {
				t.Fatalf("supported codecs = %d, want %d", got, tc.wantCount)
			}
			info := log.info.String()
			if !strings.Contains(info, fmt.Sprintf("Supported HW codecs [%d]", tc.wantCount)) {
				t.Errorf("missing codec summary in Info log: %s", info)
			}
			if tc.wantCount == 0 {
				if !strings.Contains(info, fmt.Sprintf("Codec %s", VideoCodecV264)) || !strings.Contains(info, tc.wantError) {
					t.Errorf("missing codec failure reason in Info log: %s", info)
				}
			} else {
				if strings.Contains(info, tc.wantError) {
					t.Errorf("unexpected failure output in Info log: %s", info)
				}
				if !strings.Contains(log.debug.String(), tc.wantError) {
					t.Errorf("missing failure output in Debug log: %s", log.debug.String())
				}
			}
		})
	}
}

func quietLogger(t *testing.T) {
	previous := logger.Logger
	logger.Logger = &hwProbeLogger{}
	t.Cleanup(func() { logger.Logger = previous })
}

func TestInitHWSupportProbesAMF(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STASH_TEST_HW_PROBE", "supported:h264_amf")
	t.Setenv("STASH_HW_TEST_TIMEOUT", "10")
	quietLogger(t)

	f := &FFMpeg{ffmpeg: executable}
	f.initHWSupport(context.Background())
	got := f.getHWCodecSupport()
	if len(got) != 1 || got[0] != VideoCodecA264 {
		t.Fatalf("supported codecs = %v, want [%v]", got, VideoCodecA264)
	}
}

func TestHWCodecSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		supported []VideoCodec
		// codec used for generated MP4 files
		wantGenerate VideoCodec
		// codec used for live MP4 streams, which must not change
		wantStream VideoCodec
	}{
		{"none", nil, VideoCodecLibX264, VideoCodecLibX264},
		{"vaapi", []VideoCodec{VideoCodecV264}, VideoCodecV264, VideoCodecLibX264},
		{"amf", []VideoCodec{VideoCodecA264}, VideoCodecA264, VideoCodecLibX264},
		{"nvenc", []VideoCodec{VideoCodecN264}, VideoCodecN264, VideoCodecN264},
		{"qsv before vaapi", []VideoCodec{VideoCodecI264, VideoCodecV264}, VideoCodecI264, VideoCodecI264},
		{"vaapi before amf", []VideoCodec{VideoCodecV264, VideoCodecA264}, VideoCodecV264, VideoCodecLibX264},
		{"vp9 only", []VideoCodec{VideoCodecVVP9}, VideoCodecLibX264, VideoCodecLibX264},
		{"v4l2m2m only", []VideoCodec{VideoCodecR264}, VideoCodecLibX264, VideoCodecLibX264},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &FFMpeg{hwCodecSupport: tc.supported}
			if got := f.HWCodecGenerateCompatible(VideoCodecLibX264); got != tc.wantGenerate {
				t.Errorf("generate codec = %v, want %v", got, tc.wantGenerate)
			}
			if got := f.HWCodecMP4Compatible(VideoCodecLibX264); got != tc.wantStream {
				t.Errorf("stream codec = %v, want %v", got, tc.wantStream)
			}
		})
	}
}

func TestAMFFilter(t *testing.T) {
	f := &FFMpeg{}
	for _, tc := range []struct {
		name                     string
		width, height, reqHeight int
		fullhw                   bool
		want                     VideoFilter
	}{
		{"scaled", 1920, 1080, 360, false, "format=nv12,scale=-2:360"},
		{"portrait", 1080, 1920, 360, false, "format=nv12,scale=360:-2"},
		{"not scaled", 1920, 1080, 1080, false, "format=nv12"},
		// AMF has no hardware decode path, so fullhw changes nothing
		{"fullhw", 1920, 1080, 360, true, "format=nv12,scale=-2:360"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.HWMaxResFilter(VideoCodecA264, tc.width, tc.height, tc.reqHeight, tc.fullhw); got != tc.want {
				t.Errorf("HWMaxResFilter = %q, want %q", got, tc.want)
			}
		})
	}

	if args := f.HWDeviceInit(nil, VideoCodecA264, false); len(args) != 0 {
		t.Errorf("HWDeviceInit = %v, want no device arguments", args)
	}
}

func TestAMFSkipsFullHWTest(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// the fake ffmpeg accepts any h264_amf command, so only skipping the
	// test run gives false
	t.Setenv("STASH_TEST_HW_PROBE", "supported:h264_amf")
	quietLogger(t)

	f := &FFMpeg{ffmpeg: executable}
	if f.HWCanFullHWTranscode(context.Background(), VideoCodecA264, "in.mp4", 1920, 1080, 360) {
		t.Error("HWCanFullHWTranscode(AMF) = true, want false")
	}
	// the fake accepts VAAPI the same way, which does have a full hardware path
	t.Setenv("STASH_TEST_HW_PROBE", "supported")
	if !f.HWCanFullHWTranscode(context.Background(), VideoCodecV264, "in.mp4", 1920, 1080, 360) {
		t.Error("HWCanFullHWTranscode(VAAPI) = false, want true")
	}
}
