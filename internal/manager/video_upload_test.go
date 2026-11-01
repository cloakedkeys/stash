package manager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func videoUploadConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	cfg := config.InitializeEmpty()
	root := t.TempDir()
	cfg.SetInterface(config.Stash, config.StashConfigs{{Path: root}})
	cfg.SetInterface(config.VideoExtensions, []string{"mp4", "mkv"})
	return cfg, root
}

func TestVideoUploadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix file permissions")
	}
	for _, link := range []func(string, string) error{os.Link, unsupportedUploadLink} {
		cfg, root := videoUploadConfig(t)
		path, err := saveVideoUploadWithLink(context.Background(), cfg, root, "video.mp4", strings.NewReader("video"), link)
		require.NoError(t, err)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}
}

func unsupportedUploadLink(string, string) error {
	return errors.New("hard links unsupported")
}

func TestVideoUploadLinkFallback(t *testing.T) {
	for _, scenario := range []string{"publish", "collision", "rename failure", "create failure", "link collision"} {
		t.Run(scenario, func(t *testing.T) {
			cfg, root := videoUploadConfig(t)
			dest := filepath.Join(root, "video.mp4")
			link := unsupportedUploadLink
			wantEntries := 0
			switch scenario {
			case "publish":
				wantEntries = 1
			case "collision":
				require.NoError(t, os.WriteFile(dest, []byte("original"), 0o644))
				wantEntries = 1
			case "rename failure":
				link = func(tmp, _ string) error {
					// Removing the source makes the real rename fail after reservation.
					require.NoError(t, os.Remove(tmp))
					return unsupportedUploadLink("", "")
				}
			case "link collision":
				link = func(string, string) error { return os.ErrExist }
			}
			name := "video.mp4"
			if scenario == "create failure" {
				name = strings.Repeat("v", 300) + ".mp4"
			}
			path, err := saveVideoUploadWithLink(context.Background(), cfg, root, name, strings.NewReader("video"), link)
			if scenario == "publish" {
				require.NoError(t, err)
				require.Equal(t, dest, path)
				data, err := os.ReadFile(dest)
				require.NoError(t, err)
				require.Equal(t, "video", string(data))
			} else {
				require.Error(t, err)
				require.Empty(t, path)
				if scenario == "collision" || scenario == "link collision" {
					require.ErrorContains(t, err, "already exists; nothing was overwritten")
				}
				if scenario == "rename failure" {
					require.ErrorContains(t, err, "renaming upload")
				}
				if scenario == "create failure" {
					require.ErrorContains(t, err, "reserving upload destination")
				}
				if scenario == "collision" {
					data, err := os.ReadFile(dest)
					require.NoError(t, err)
					require.Equal(t, "original", string(data))
				}
			}
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, wantEntries)
		})
	}
}

func TestVideoUploadRejectsPathsAndNames(t *testing.T) {
	cfg, root := videoUploadConfig(t)
	for _, path := range []string{"", t.TempDir(), filepath.Join(root, "subfolder"), root + "-other"} {
		_, err := saveVideoUpload(context.Background(), cfg, path, "video.mp4", strings.NewReader("video"))
		require.Error(t, err, path)
	}
	cfg.SetInterface(config.Stash, config.StashConfigs{{Path: root, ExcludeVideo: true}})
	_, err := saveVideoUpload(context.Background(), cfg, root, "video.mp4", strings.NewReader("video"))
	require.ErrorContains(t, err, "does not allow video")
	cfg.SetInterface(config.Stash, config.StashConfigs{{Path: root}})
	for _, name := range []string{"", ".", "..", "../escape.mp4", `..\escape.mp4`, "sub/video.mp4", `sub\video.mp4`, "/video.mp4", `C:\video.mp4`, "CON.mp4", "aux.mp4", "PRN.mp4", "NUL.mp4", "COM1.mp4", "lpt9.mp4", "COM¹.mp4", "CON .mp4", "CONIN$.mp4", "a:b.mp4", "a?.mp4", "a*.mp4", "a|b.mp4", "a<b.mp4", "a>b.mp4", "a\"b.mp4", "a\x00.mp4", "a\n.mp4", "video.mp4.", "video.mp4 ", "video.exe", "video", "video.jpg", "video.zip", "video.mp3"} {
		_, err := saveVideoUpload(context.Background(), cfg, root, name, strings.NewReader("video"))
		require.Error(t, err, name)
	}
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)

	// The same Clean normalisation used by library configuration is accepted.
	path, err := saveVideoUpload(context.Background(), cfg, root+string(os.PathSeparator)+".", "video.MP4", strings.NewReader("video"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "video.MP4"), path)
}

func TestVideoUploadCollisionNeverOverwrites(t *testing.T) {
	cfg, root := videoUploadConfig(t)
	dest := filepath.Join(root, "video.mp4")
	require.NoError(t, os.WriteFile(dest, []byte("original"), 0600))
	_, err := saveVideoUpload(context.Background(), cfg, root, "video.mp4", strings.NewReader("replacement"))
	require.ErrorContains(t, err, "already exists")
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

// Both writers finish their temporary file before either can publish it.
type racingUploadReader struct {
	io.Reader
	ready *sync.WaitGroup
	start <-chan struct{}
	once  sync.Once
}

func (r *racingUploadReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.once.Do(func() { r.ready.Done(); <-r.start })
	}
	return n, err
}

func TestVideoUploadRace(t *testing.T) {
	t.Run("hard link", func(t *testing.T) { testVideoUploadRace(t, os.Link) })
	t.Run("fallback", func(t *testing.T) { testVideoUploadRace(t, unsupportedUploadLink) })
}

func testVideoUploadRace(t *testing.T, link func(string, string) error) {
	for _, sameName := range []bool{true, false} {
		t.Run(map[bool]string{true: "same name", false: "different names"}[sameName], func(t *testing.T) {
			cfg, root := videoUploadConfig(t)
			var ready sync.WaitGroup
			ready.Add(2)
			start := make(chan struct{})
			errs := make(chan error, 2)
			for i, content := range []string{"first", "second"} {
				name := "video.mp4"
				if !sameName && i == 1 {
					name = "other.mp4"
				}
				go func() {
					_, err := saveVideoUploadWithLink(context.Background(), cfg, root, name, &racingUploadReader{Reader: strings.NewReader(content), ready: &ready, start: start}, link)
					errs <- err
				}()
			}
			ready.Wait()
			close(start)
			failures := 0
			for range 2 {
				if err := <-errs; err != nil {
					require.ErrorContains(t, err, "already exists")
					failures++
				}
			}
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			if sameName {
				require.Equal(t, 1, failures)
				require.Len(t, entries, 1)
				data, err := os.ReadFile(filepath.Join(root, "video.mp4"))
				require.NoError(t, err)
				require.Contains(t, []string{"first", "second"}, string(data))
			} else {
				require.Zero(t, failures)
				require.Len(t, entries, 2)
				for name, want := range map[string]string{"video.mp4": "first", "other.mp4": "second"} {
					data, err := os.ReadFile(filepath.Join(root, name))
					require.NoError(t, err)
					require.Equal(t, want, string(data))
				}
			}
		})
	}
}

type failedUploadReader struct{}

func (failedUploadReader) Read(p []byte) (int, error) {
	return copy(p, "partial"), errors.New("broken upload")
}

func TestVideoUploadCleansUpFailures(t *testing.T) {
	for _, failure := range []string{"read", "cancelled", "too large", "publish"} {
		t.Run(failure, func(t *testing.T) {
			cfg, root := videoUploadConfig(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var src io.Reader = strings.NewReader("video")
			want := 0
			switch failure {
			case "read":
				src = failedUploadReader{}
			case "cancelled":
				cancel()
			case "too large":
				cfg.SetInt(config.MaxUploadSize, 1)
				src = strings.NewReader(strings.Repeat("v", (1<<20)+1))
			case "publish":
				// A directory collision must also leave the destination intact.
				require.NoError(t, os.Mkdir(filepath.Join(root, "video.mp4"), 0700))
				want = 1
			}
			_, err := saveVideoUpload(ctx, cfg, root, "video.mp4", src)
			require.Error(t, err)
			if failure == "too large" {
				require.ErrorContains(t, err, "exceeds max_upload_size")
			}
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, want)
		})
	}
}

func TestVideoUploadScansOnlySavedFile(t *testing.T) {
	for _, scanFails := range []bool{false, true} {
		cfg, root := videoUploadConfig(t)
		dest := filepath.Join(root, "video.mp4")
		calls := 0
		video := &models.VideoFile{}
		scan := func(ctx context.Context, input ScanFileInput) (models.File, error) {
			calls++
			require.Equal(t, ScanFileInput{Path: dest}, input)
			data, err := os.ReadFile(dest)
			require.NoError(t, err)
			require.Equal(t, "video", string(data))
			if scanFails {
				return nil, errors.New("probe failed")
			}
			return video, nil
		}
		ret, err := uploadVideo(context.Background(), cfg, root, "video.mp4", strings.NewReader("video"), scan)
		if scanFails {
			require.ErrorContains(t, err, "file was saved at")
			require.ErrorContains(t, err, dest)
			require.ErrorContains(t, err, "probe failed")
		} else {
			require.NoError(t, err)
			require.Same(t, video, ret)
		}
		require.Equal(t, 1, calls)
		entries, err := os.ReadDir(root)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		// A failed save cannot reach the scanner.
		_, err = uploadVideo(context.Background(), cfg, root, "video.mp4", strings.NewReader("new"), scan)
		require.ErrorContains(t, err, "already exists")
		require.Equal(t, 1, calls)
	}
}
