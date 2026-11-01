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
		if mode == "supported" {
			for i, arg := range os.Args {
				if arg == "-c:v" && i+1 < len(os.Args) && os.Args[i+1] == "h264_vaapi" {
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
