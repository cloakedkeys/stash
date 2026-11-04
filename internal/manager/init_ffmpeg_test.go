package manager

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/logger"
)

const fakeFFMpegEnv = "STASH_TEST_FAKE_FFMPEG"

// Run the test executable as a fake ffmpeg or ffprobe. It succeeds for the
// "-h" check that ValidateFFMpeg and ValidateFFProbe run, and fails anything
// else, such as the hardware encoder probes.
func TestMain(m *testing.M) {
	if os.Getenv(fakeFFMpegEnv) != "" {
		for _, arg := range os.Args[1:] {
			if arg == "-h" {
				os.Exit(0)
			}
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

type ffmpegPathLogger struct {
	logger.BasicLogger
	mu     sync.Mutex
	errors strings.Builder
}

func (l *ffmpegPathLogger) Errorf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(&l.errors, format+"\n", args...)
}

func (l *ffmpegPathLogger) Info(...interface{})           {}
func (l *ffmpegPathLogger) Infof(string, ...interface{})  {}
func (l *ffmpegPathLogger) Debug(...interface{})          {}
func (l *ffmpegPathLogger) Debugf(string, ...interface{}) {}

func (l *ffmpegPathLogger) errorText() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.errors.String()
}

type ffmpegTestEnv struct {
	dir     string
	fake    string // runs as a working ffmpeg and ffprobe
	missing string // does not exist
	notExec string // exists, but is not a program
}

func setupFFMpegTest(t *testing.T) (*ffmpegTestEnv, *ffmpegPathLogger) {
	t.Helper()

	fake, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeFFMpegEnv, "1")

	dir := t.TempDir()
	notExec := filepath.Join(dir, "not-a-program.txt")
	if err := os.WriteFile(notExec, []byte("not a program"), 0644); err != nil {
		t.Fatal(err)
	}

	log := &ffmpegPathLogger{}
	previous := logger.Logger
	logger.Logger = log
	t.Cleanup(func() { logger.Logger = previous })

	return &ffmpegTestEnv{
		dir:     dir,
		fake:    fake,
		missing: filepath.Join(dir, "missing", "ffprobe.exe"),
		notExec: notExec,
	}, log
}

func newFFMpegTestManager(t *testing.T, dir, ffmpegPath, ffprobePath string) *Manager {
	t.Helper()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(dir, "config.yml"))
	cfg.SetString(config.FFMpegPath, ffmpegPath)
	cfg.SetString(config.FFProbePath, ffprobePath)
	return &Manager{Config: cfg}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	// a copy, not a hard link: Windows will not delete a link to a running
	// program, so the test directory could not be cleaned up
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// A configured ffprobe path that cannot be run must be refused the same way
// a configured ffmpeg path already is. Before the fix, the ffprobe path was
// never checked: the ffmpeg path was checked in its place.
func TestRefreshFFMpegRejectsInvalidConfiguredPath(t *testing.T) {
	env, _ := setupFFMpegTest(t)

	for _, tc := range []struct {
		name        string
		ffmpegPath  string
		ffprobePath string
		wantLog     string
	}{
		{"missing ffmpeg", env.missing, env.fake, "invalid ffmpeg path: error running ffmpeg"},
		{"missing ffprobe", env.fake, env.missing, "invalid ffprobe path: error running ffprobe"},
		{"ffmpeg not a program", env.notExec, env.fake, "invalid ffmpeg path: error running ffmpeg"},
		{"ffprobe not a program", env.fake, env.notExec, "invalid ffprobe path: error running ffprobe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &ffmpegPathLogger{}
			logger.Logger = log

			mgr := newFFMpegTestManager(t, env.dir, tc.ffmpegPath, tc.ffprobePath)
			mgr.RefreshFFMpeg(context.Background())

			if mgr.FFMpeg != nil {
				t.Errorf("FFMpeg = %q, want nil", mgr.FFMpeg.Path())
			}
			if mgr.FFProbe != nil {
				t.Errorf("FFProbe = %q, want nil", mgr.FFProbe.Path())
			}
			if got := log.errorText(); !strings.Contains(got, tc.wantLog) {
				t.Errorf("error log = %q, want it to contain %q", got, tc.wantLog)
			}
		})
	}
}

// On a refresh, an invalid configured path leaves the ffmpeg and ffprobe in
// use unchanged, for ffprobe the same as for ffmpeg.
func TestRefreshFFMpegInvalidPathKeepsPrevious(t *testing.T) {
	env, _ := setupFFMpegTest(t)

	for _, tc := range []struct {
		name        string
		ffmpegPath  string
		ffprobePath string
	}{
		{"missing ffmpeg", env.missing, env.fake},
		{"missing ffprobe", env.fake, env.missing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newFFMpegTestManager(t, env.dir, tc.ffmpegPath, tc.ffprobePath)
			previousFFMpeg := ffmpeg.NewEncoder("previous-ffmpeg")
			previousFFProbe := ffmpeg.NewFFProbe("previous-ffprobe")
			mgr.FFMpeg = previousFFMpeg
			mgr.FFProbe = previousFFProbe

			mgr.RefreshFFMpeg(context.Background())

			if mgr.FFMpeg != previousFFMpeg {
				t.Errorf("FFMpeg changed to %q", mgr.FFMpeg.Path())
			}
			if mgr.FFProbe != previousFFProbe {
				t.Errorf("FFProbe changed to %q", mgr.FFProbe.Path())
			}
		})
	}
}

// A valid configured ffprobe path is used as is.
func TestRefreshFFMpegUsesValidConfiguredPaths(t *testing.T) {
	env, log := setupFFMpegTest(t)

	mgr := newFFMpegTestManager(t, env.dir, env.fake, env.fake)
	mgr.RefreshFFMpeg(context.Background())

	if mgr.FFMpeg == nil || mgr.FFMpeg.Path() != env.fake {
		t.Errorf("FFMpeg = %v, want %q", mgr.FFMpeg, env.fake)
	}
	if mgr.FFProbe == nil || mgr.FFProbe.Path() != env.fake {
		t.Errorf("FFProbe = %v, want %q", mgr.FFProbe, env.fake)
	}
	if got := log.errorText(); got != "" {
		t.Errorf("unexpected error log: %q", got)
	}
}

// An empty ffprobe path is still found automatically, starting with the
// configuration directory.
func TestRefreshFFMpegResolvesEmptyFFProbePath(t *testing.T) {
	env, log := setupFFMpegTest(t)

	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name = "ffprobe.exe"
	}
	resolved := filepath.Join(env.dir, name)
	copyFile(t, env.fake, resolved)

	mgr := newFFMpegTestManager(t, env.dir, env.fake, "")
	mgr.RefreshFFMpeg(context.Background())

	if mgr.FFProbe == nil || mgr.FFProbe.Path() != resolved {
		t.Errorf("FFProbe = %v, want %q", mgr.FFProbe, resolved)
	}
	if mgr.FFMpeg == nil || mgr.FFMpeg.Path() != env.fake {
		t.Errorf("FFMpeg = %v, want %q", mgr.FFMpeg, env.fake)
	}
	if got := log.errorText(); got != "" {
		t.Errorf("unexpected error log: %q", got)
	}
}
