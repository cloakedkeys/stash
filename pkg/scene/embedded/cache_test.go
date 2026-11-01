package embedded

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	stashExec "github.com/stashapp/stash/pkg/exec"
	"github.com/stretchr/testify/require"
)

const probeOutput = `{"streams":[
 {"index":0,"codec_type":"video","codec_name":"subrip"},
 {"index":1,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle"},
 {"index":2,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"eng","title":"Dialogue"}},
 {"index":3,"codec_type":"subtitle","codec_name":"ass","tags":{"language":"eng","title":"Commentary"}},
 {"index":4,"codec_type":"subtitle","codec_name":"ssa"},
 {"index":5,"codec_type":"subtitle","codec_name":"webvtt"},
 {"index":6,"codec_type":"subtitle","codec_name":"mov_text"},
 {"index":7,"codec_type":"subtitle","codec_name":"dvd_subtitle"},
 {"index":8,"codec_type":"subtitle","codec_name":"unknown"}
 ]}`

func testCache(t *testing.T) Cache {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "movie.mkv")
	require.NoError(t, os.WriteFile(source, []byte("media"), 0600))
	return Cache{Dir: filepath.Join(dir, "generated"), Source: source, FileID: "123",
		probe: func(context.Context) ([]byte, error) { return []byte(probeOutput), nil },
		extract: func(_ context.Context, index int, tmp string) error {
			return os.WriteFile(tmp, []byte(fmt.Sprintf("WEBVTT\n\n%d", index)), 0600)
		},
	}
}

func TestTextTrackSelectionAndIndex(t *testing.T) {
	tracks, err := SelectTracks([]byte(probeOutput))
	require.NoError(t, err)
	require.Equal(t, []Track{{2, "eng", "Dialogue", "subrip"}, {3, "eng", "Commentary", "ass"}, {4, "", "", "ssa"}, {5, "", "", "webvtt"}, {6, "", "", "mov_text"}}, tracks)
	_, err = SelectTracks([]byte("invalid"))
	require.Error(t, err)
	c := testCache(t)
	c.extract = func(context.Context, int, string) error { t.Error("invalid stream extracted"); return nil }
	for _, index := range []int{-1, 0, 1, 7, 8, 999} {
		_, err := c.Extract(context.Background(), index)
		require.ErrorIs(t, err, ErrTrackNotFound)
	}
}

func TestCacheInvalidatesWhenFileChanges(t *testing.T) {
	c := testCache(t)
	var probes atomic.Int32
	c.probe = func(context.Context) ([]byte, error) { probes.Add(1); return []byte(probeOutput), nil }
	first, err := c.Extract(context.Background(), 2)
	require.NoError(t, err)
	// Simulate reopening: a new Cache value must use the on-disk probe result.
	reopened := c
	reopened.probe = func(context.Context) ([]byte, error) {
		t.Error("probed cached version")
		return nil, errors.New("unexpected probe")
	}
	again, err := reopened.Extract(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, first, again)
	v1, err := Version(c.FileID, c.Source)
	require.NoError(t, err)
	info, err := os.Stat(c.Source)
	require.NoError(t, err)
	newer := info.ModTime().Add(time.Second)
	require.NoError(t, os.Chtimes(c.Source, newer, newer))
	v2, err := Version(c.FileID, c.Source)
	require.NoError(t, err)
	require.NotEqual(t, v1, v2)
	second, err := c.Extract(context.Background(), 2)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.EqualValues(t, 2, probes.Load())
	third, err := c.Extract(context.Background(), 3)
	require.NoError(t, err)
	require.NotEqual(t, second, third)
	identity, err := Version("different file", c.Source)
	require.NoError(t, err)
	require.NotEqual(t, v2, identity)
}

func TestFailedExtractionIsNotPublished(t *testing.T) {
	c := testCache(t)
	c.extract = func(_ context.Context, _ int, tmp string) error {
		require.NoError(t, os.WriteFile(tmp, []byte("partial"), 0600))
		return errors.New("encoder failed")
	}
	_, err := c.Extract(context.Background(), 2)
	require.ErrorContains(t, err, "encoder failed")
	entries, err := os.ReadDir(c.Dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.True(t, strings.HasSuffix(entries[0].Name(), ".json"))
	c.extract = func(_ context.Context, _ int, tmp string) error { return os.WriteFile(tmp, []byte("WEBVTT"), 0600) }
	_, err = c.Extract(context.Background(), 2)
	require.NoError(t, err)
}

func TestConcurrentRequestsShareExtraction(t *testing.T) {
	c := testCache(t)
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	c.extract = func(ctx context.Context, _ int, tmp string) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
		}
		return os.WriteFile(tmp, []byte("WEBVTT"), 0600)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { _, err := c.Extract(ctx, 2); first <- err }()
	<-started
	second := make(chan error, 1)
	go func() { _, err := c.Extract(context.Background(), 2); second <- err }()
	require.Eventually(t, func() bool {
		work.Lock()
		defer work.Unlock()
		for key, j := range work.jobs {
			if strings.HasSuffix(key, "-2.vtt") && strings.HasPrefix(key, c.Dir) && j.users == 2 {
				return true
			}
		}
		return false
	}, time.Second*5, time.Millisecond)
	// One caller leaving must not cancel work the second caller still needs.
	cancel()
	require.ErrorIs(t, <-first, context.Canceled)
	close(release)
	require.NoError(t, <-second)
	require.EqualValues(t, 1, calls.Load())
}

func TestLastRequestCancelsExtraction(t *testing.T) {
	c := testCache(t)
	started, stopped := make(chan struct{}), make(chan struct{})
	c.extract = func(ctx context.Context, _ int, tmp string) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Extract(ctx, 2); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	select {
	case <-stopped:
	case <-time.After(time.Second * 5):
		t.Fatal("extraction not cancelled")
	}
	require.Eventually(t, func() bool {
		entries, err := os.ReadDir(c.Dir)
		return err == nil && len(entries) == 1 && strings.HasSuffix(entries[0].Name(), ".json")
	}, time.Second*5, time.Millisecond)
}

func TestDeletingGeneratedDirectoryCancelsPublication(t *testing.T) {
	c := testCache(t)
	started, release := make(chan struct{}), make(chan struct{})
	c.extract = func(ctx context.Context, _ int, tmp string) error {
		close(started)
		<-release
		// Even a process that exits successfully as cancellation arrives must
		// never publish into a directory removed by the cleanup transaction.
		return nil
	}
	done := make(chan error, 1)
	go func() { _, err := c.Extract(context.Background(), 2); done <- err }()
	<-started
	require.NoError(t, MarkForDeletion(c.Dir, func() error { return os.Rename(c.Dir, c.Dir+".delete") }))
	close(release)
	require.ErrorIs(t, <-done, context.Canceled)
	require.NoDirExists(t, c.Dir)
	matches, err := filepath.Glob(filepath.Join(c.Dir+".delete", "*.vtt"))
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestExtractionConcurrencyIsLimited(t *testing.T) {
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var active, maximum atomic.Int32
	caches := make([]Cache, 4)
	for i := range caches {
		caches[i] = testCache(t)
		_, err := caches[i].Tracks(context.Background())
		require.NoError(t, err)
		caches[i].extract = func(ctx context.Context, _ int, tmp string) error {
			n := active.Add(1)
			defer active.Add(-1)
			for old := maximum.Load(); n > old; old = maximum.Load() {
				if maximum.CompareAndSwap(old, n) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return os.WriteFile(tmp, []byte("WEBVTT"), 0600)
		}
	}
	done := make(chan error, len(caches))
	for _, c := range caches {
		go func(c Cache) { _, err := c.Extract(context.Background(), 2); done <- err }(c)
	}
	<-started
	<-started
	require.EqualValues(t, 2, active.Load())
	close(release)
	for range caches {
		require.NoError(t, <-done)
	}
	require.EqualValues(t, 2, maximum.Load())
}

func TestProbeDoesNotWaitForExtractionSlots(t *testing.T) {
	started := make(chan struct{}, cap(work.slots))
	release := make(chan struct{})
	done := make(chan error, cap(work.slots))
	launched := 0
	defer func() {
		close(release)
		for i := 0; i < launched; i++ {
			require.NoError(t, <-done)
		}
	}()
	for i := 0; i < cap(work.slots); i++ {
		c := testCache(t)
		_, err := c.Tracks(context.Background())
		require.NoError(t, err)
		c.extract = func(ctx context.Context, _ int, tmp string) error {
			started <- struct{}{}
			select {
			case <-release:
				return os.WriteFile(tmp, []byte("WEBVTT"), 0600)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		go func() { _, err := c.Extract(context.Background(), 2); done <- err }()
		launched++
	}
	for i := 0; i < cap(work.slots); i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("extraction did not start")
		}
	}

	c := testCache(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tracks, err := c.Tracks(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, tracks)
}

func TestRealFFMpegExtraction(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "input.srt")
	require.NoError(t, os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nHello embedded\n"), 0600))
	source := filepath.Join(dir, "movie.mkv")
	output, err := stashExec.Command(ffmpeg, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=size=16x16:duration=1", "-i", srt, "-map", "0:v", "-map", "1:0", "-c:v", "ffv1", "-c:s", "srt", "-metadata:s:s:0", "language=eng", "-metadata:s:s:0", "title=Dialogue", source).CombinedOutput()
	require.NoError(t, err, string(output))
	c := Cache{Dir: filepath.Join(dir, "generated"), Source: source, FileID: "1", FFMpeg: ffmpeg, FFProbe: ffprobe}
	tracks, err := c.Tracks(context.Background())
	require.NoError(t, err)
	require.Equal(t, []Track{{1, "eng", "Dialogue", "subrip"}}, tracks)
	target, err := c.Extract(context.Background(), 1)
	require.NoError(t, err)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Contains(t, string(data), "WEBVTT")
	require.Contains(t, string(data), "Hello embedded")
}
