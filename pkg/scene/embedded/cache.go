// Package embedded provides on-demand, generated text subtitle caches.
package embedded

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	stashExec "github.com/stashapp/stash/pkg/exec"
)

var ErrTrackNotFound = errors.New("embedded text track not found")

type Track struct {
	StreamIndex  int    `json:"stream_index"`
	LanguageCode string `json:"language_code"`
	Title        string `json:"title"`
	Codec        string `json:"codec"`
}

// SelectTracks deliberately accepts only text subtitle codecs.
func SelectTracks(data []byte) ([]Track, error) {
	var probe struct {
		Streams []struct {
			Index     int    `json:"index"`
			CodecName string `json:"codec_name"`
			CodecType string `json:"codec_type"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}
	ret := []Track{}
	for _, s := range probe.Streams {
		if s.CodecType != "subtitle" || s.Index < 0 {
			continue
		}
		switch s.CodecName {
		case "subrip", "ass", "ssa", "webvtt", "mov_text":
			ret = append(ret, Track{s.Index, s.Tags.Language, s.Tags.Title, s.CodecName})
		}
	}
	return ret, nil
}

// Version uses the database file identity and current filesystem metadata, not
// the scan-time timestamp. A changed file invalidates both caches.
func Version(fileID, source string) (string, error) {
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", fileID, source, info.Size(), info.ModTime().UnixNano())))
	return fmt.Sprintf("%x", sum), nil
}

type job struct {
	done      chan struct{}
	cancel    context.CancelFunc
	users     int
	err       error
	abandoned bool
}

var work = struct {
	sync.Mutex
	jobs  map[string]*job
	slots chan struct{}
}{jobs: make(map[string]*job), slots: make(chan struct{}, 2)}

// MarkForDeletion cancels work for this scene and serializes the directory move
// with temporary-file creation and publication. It never waits for a process.
func MarkForDeletion(dir string, mark func() error) error {
	work.Lock()
	defer work.Unlock()
	prefix := filepath.Clean(dir) + string(os.PathSeparator)
	for target, j := range work.jobs {
		if strings.HasPrefix(target, prefix) {
			j.abandoned = true
			j.cancel()
		}
	}
	return mark()
}

// cached coalesces callers. Work survives one caller leaving while another still
// needs it, and is cancelled as soon as the last caller leaves.
func cached(ctx context.Context, target string, generate func(context.Context, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	work.Lock()
	j := work.jobs[target]
	if j != nil && j.abandoned {
		work.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-j.done:
			return cached(ctx, target, generate)
		}
	}
	if j == nil {
		j = &job{done: make(chan struct{})}
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		j.cancel = cancel
		work.jobs[target] = j
		go func() {
			defer cancel()
			err := publish(runCtx, target, generate)
			work.Lock()
			j.err = err
			if work.jobs[target] == j {
				delete(work.jobs, target)
			}
			close(j.done)
			work.Unlock()
		}()
	}
	j.users++
	work.Unlock()
	defer func() {
		work.Lock()
		defer work.Unlock()
		j.users--
		if j.users == 0 {
			j.cancel()
			j.abandoned = true
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-j.done:
		return j.err
	}
}

func publish(ctx context.Context, target string, generate func(context.Context, string) error) error {
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	work.Lock()
	tmp, err := createTemp(ctx, filepath.Dir(target))
	work.Unlock()
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	defer os.Remove(name)
	if err := generate(ctx, name); err != nil {
		return err
	}
	work.Lock()
	defer work.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(name, target)
}

func createTemp(ctx context.Context, dir string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, ".subtitle-*")
}

// Cache is scoped to a scene's generated directory. Callers resolve the primary
// file before entering here, so processes run outside database transactions.
type Cache struct {
	Dir, Source, FileID, FFProbe, FFMpeg string
	probe                                func(context.Context) ([]byte, error)
	extract                              func(context.Context, int, string) error
}

func (c Cache) tracks(ctx context.Context, version string) ([]Track, error) {
	target := filepath.Join(c.Dir, version+".json")
	err := cached(ctx, target, func(ctx context.Context, tmp string) error {
		data, err := c.probeFile(ctx)
		if err != nil {
			return err
		}
		tracks, err := SelectTracks(data)
		if err != nil {
			return err
		}
		data, err = json.Marshal(tracks)
		if err != nil {
			return err
		}
		if err := c.unchanged(version); err != nil {
			return err
		}
		return os.WriteFile(tmp, data, 0600)
	})
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	var tracks []Track
	err = json.Unmarshal(data, &tracks)
	return tracks, err
}

func (c Cache) unchanged(version string) error {
	now, err := Version(c.FileID, c.Source)
	if err != nil {
		return err
	}
	if now != version {
		return errors.New("subtitle source changed during generation")
	}
	return nil
}

func (c Cache) Tracks(ctx context.Context) ([]Track, error) {
	version, err := Version(c.FileID, c.Source)
	if err != nil {
		return nil, err
	}
	return c.tracks(ctx, version)
}

func (c Cache) Extract(ctx context.Context, index int) (string, error) {
	version, err := Version(c.FileID, c.Source)
	if err != nil {
		return "", err
	}
	tracks, err := c.tracks(ctx, version)
	if err != nil {
		return "", err
	}
	found := false
	for _, t := range tracks {
		if t.StreamIndex == index {
			found = true
			break
		}
	}
	if !found {
		return "", ErrTrackNotFound
	}
	target := filepath.Join(c.Dir, version+"-"+strconv.Itoa(index)+".vtt")
	err = cached(ctx, target, func(ctx context.Context, tmp string) error {
		select {
		case work.slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-work.slots }()
		// The map argument is built only from a validated integer, never query text.
		if err := c.extractFile(ctx, index, tmp); err != nil {
			return err
		}
		return c.unchanged(version)
	})
	if err != nil {
		return "", err
	}
	if err := c.unchanged(version); err != nil {
		return "", err
	}
	return target, nil
}

func (c Cache) probeFile(ctx context.Context) ([]byte, error) {
	if c.probe != nil {
		return c.probe(ctx)
	}
	return stashExec.CommandContext(ctx, c.FFProbe, "-v", "error", "-show_streams", "-of", "json", "-i", c.Source).Output()
}

func (c Cache) extractFile(ctx context.Context, index int, tmp string) error {
	if c.extract != nil {
		return c.extract(ctx, index, tmp)
	}
	return stashExec.CommandContext(ctx, c.FFMpeg, "-nostdin", "-v", "error", "-y", "-i", c.Source, "-map", "0:"+strconv.Itoa(index), "-c:s", "webvtt", "-f", "webvtt", tmp).Run()
}
