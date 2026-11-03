//go:build integration

package manager

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/audio"
	"github.com/stashapp/stash/pkg/file"
	imagepkg "github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/sqlite"
	_ "github.com/stashapp/stash/pkg/sqlite/migrations"
	"github.com/stretchr/testify/require"
)

// These helpers read real GIF metadata and checksums without an external
// FFmpeg process. The scanner, media handlers, database and clean are real.
// Animated GIFs exercise the same VideoFile/image overlap as WebM clips.
type cleanMediaGIFReader struct{}

func (cleanMediaGIFReader) Decorate(ctx context.Context, fs models.FS, f models.File) (models.File, error) {
	r, err := fs.Open(f.Base().Path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	g, err := gif.DecodeAll(r)
	if err != nil {
		return nil, err
	}
	duration := 0.0
	for _, delay := range g.Delay {
		duration += float64(delay) / 100
	}
	return &models.VideoFile{
		BaseFile: f.Base(), Format: "gif", VideoCodec: "gif",
		Width: g.Config.Width, Height: g.Config.Height, Duration: duration,
	}, nil
}

func (cleanMediaGIFReader) IsMissingMetadata(ctx context.Context, fs models.FS, f models.File) bool {
	_, ok := f.(*models.VideoFile)
	return !ok
}

// cleanMediaAudioReader stands in for FFprobe on a file scanned as audio.
type cleanMediaAudioReader struct{}

func (cleanMediaAudioReader) Decorate(ctx context.Context, fs models.FS, f models.File) (models.File, error) {
	return &models.AudioFile{BaseFile: f.Base(), Format: "gif", AudioCodec: "aac", Duration: 0.5, SampleRate: 44100, BitRate: 128000}, nil
}

func (cleanMediaAudioReader) IsMissingMetadata(ctx context.Context, fs models.FS, f models.File) bool {
	_, ok := f.(*models.AudioFile)
	return !ok
}

type cleanMediaFingerprints struct{}

func (cleanMediaFingerprints) CalculateFingerprints(f *models.BaseFile, opener file.Opener, useExisting bool) ([]models.Fingerprint, error) {
	r, err := opener.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	h := md5.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return []models.Fingerprint{{Type: models.FingerprintTypeMD5, Fingerprint: fmt.Sprintf("%x", h.Sum(nil))}}, nil
}

func cleanMediaManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	library := filepath.Join(dir, "library")
	require.NoError(t, os.MkdirAll(library, 0755))
	// the database's migrations read the config, so it comes first
	previous := instance
	config.InitializeEmpty()
	cfg := config.GetInstance()
	cfg.SetInterface(config.Stash, config.StashConfigs{{Path: library}})
	cfg.SetInterface(config.Generated, filepath.Join(dir, "generated"))
	cfg.SetInterface(config.VideoExtensions, []string{"gif"})
	cfg.SetInterface(config.ImageExtensions, []string{"gif"})
	cfg.SetInterface(config.VideoFileNamingAlgorithm, string(models.HashAlgorithmMd5))
	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(context.Background(), filepath.Join(dir, "test.sqlite")))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	p := paths.NewPaths(cfg.GetGeneratedPath(), "")
	r := db.Repository()
	mgr := &Manager{Config: cfg, Repository: r, Paths: &p, PluginCache: plugin.NewCache(cfg)}
	mgr.SceneService = &scene.Service{Repository: r.Scene, File: r.File, MarkerRepository: r.SceneMarker, Paths: &p, Config: cfg}
	mgr.ImageService = &imagepkg.Service{Repository: r.Image, File: r.File}
	instance = mgr
	t.Cleanup(func() { instance = previous })
	return mgr, library
}

func cleanMediaGIF(t *testing.T) []byte {
	t.Helper()
	frame := image.NewPaletted(image.Rect(0, 0, 12, 8), color.Palette{color.Black, color.White})
	var out bytes.Buffer
	require.NoError(t, gif.EncodeAll(&out, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{20, 30}}))
	return out.Bytes()
}

func scanCleanMedia(t *testing.T, mgr *Manager, library, path string) models.File {
	t.Helper()
	ctx := context.Background()
	handlers := getScanHandlers(ScanGenerateOptions{}, nil, nil)
	for _, h := range handlers {
		// tags and covers are read with FFprobe, which these tests don't run
		if fh, ok := h.(*file.FilteredHandler); ok {
			if ah, ok := fh.Handler.(*audio.ScanHandler); ok {
				ah.MetadataReader = nil
			}
		}
	}
	scanner := &file.Scanner{
		FS: &file.OsFS{}, Repository: file.NewRepository(mgr.Repository),
		RootPaths: []string{library}, ZipFileExtensions: mgr.Config.GetGalleryExtensions(),
		FingerprintCalculator: cleanMediaFingerprints{},
		ScanFilters:           []file.PathFilter{newScanFilter(mgr.Config, mgr.Repository, time.Time{})},
		FileDecorators: []file.Decorator{&file.FilteredDecorator{
			Decorator: cleanMediaGIFReader{}, Filter: file.FilterFunc(func(ctx context.Context, f models.File) bool {
				return videoFileFilter(ctx, f) || imageFileFilter(ctx, f)
			}),
		}, &file.FilteredDecorator{
			Decorator: cleanMediaAudioReader{}, Filter: file.FilterFunc(audioFileFilter),
		}},
		FileHandlers: handlers,
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, scanner.AcceptEntry(ctx, path, info, ""))
	ff, err := makeScannedFile(scanner.FS, path, info, nil)
	require.NoError(t, err)
	j := ScanJob{scanner: scanner}
	require.NoError(t, j.handleFile(ctx, *ff, nil))
	var scanned models.File
	require.NoError(t, mgr.Repository.WithReadTxn(ctx, func(ctx context.Context) error {
		var err error
		scanned, err = mgr.Repository.File.FindByPath(ctx, path, true)
		return err
	}))
	require.NotNil(t, scanned)
	return scanned
}

func runMediaClean(t *testing.T, mgr *Manager, options file.VerifyOptions) {
	t.Helper()
	j := &verifyJob{
		verifier: &file.Verifier{
			FS: &file.OsFS{}, Repository: file.NewRepository(mgr.Repository),
			PurgeHandlers: []file.PurgeHandler{&purgeHandler{}},
		},
		repository: mgr.Repository, options: options, scanSubs: &subscriptionManager{},
	}
	result := make(chan error, 1)
	jobs := job.NewManager()
	jobs.Start(context.Background(), "clean", job.MakeJobExec(func(ctx context.Context, progress *job.Progress) error {
		err := j.Execute(ctx, progress)
		result <- err
		return err
	}))
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("clean did not finish")
	}
}

func requireCleanMediaCounts(t *testing.T, mgr *Manager, id models.FileID, scenes, images int) {
	t.Helper()
	require.NoError(t, mgr.Repository.WithReadTxn(context.Background(), func(ctx context.Context) error {
		files, err := mgr.Repository.File.Find(ctx, id)
		require.NoError(t, err)
		require.Len(t, files, 1, "the shared file entry must remain")
		require.Nil(t, files[0].Base().MissingSince)
		ss, err := mgr.Repository.Scene.FindByFileID(ctx, id)
		require.NoError(t, err)
		require.Len(t, ss, scenes)
		ii, err := mgr.Repository.Image.FindByFileID(ctx, id)
		require.NoError(t, err)
		require.Len(t, ii, images)
		return nil
	}))
}

// A .webm moved from the video to the image extensions stays in the library as
// an image, and Clean must remove the scene made from it (#1533). An animated
// GIF stands in for the .webm: it is a video file that can also be an image.
func TestCleanRemovesScenesOfFileNoLongerVideo(t *testing.T) {
	for _, setting := range []string{"extension", "pattern", "library"} {
		t.Run(setting, func(t *testing.T) {
			mgr, library := cleanMediaManager(t)
			path := filepath.Join(library, "clip.gif")
			content := cleanMediaGIF(t)
			require.NoError(t, os.WriteFile(path, content, 0644))
			f := scanCleanMedia(t, mgr, library, path)
			require.IsType(t, &models.VideoFile{}, f)
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)

			cfg := mgr.Config
			switch setting {
			case "extension":
				cfg.SetInterface(config.VideoExtensions, []string{"mp4"})
			case "pattern":
				cfg.SetInterface(config.Exclude, []string{`\.gif$`})
			case "library":
				cfg.SetInterface(config.Stash, config.StashConfigs{{Path: library, ExcludeVideo: true}})
			}

			// a clean of another folder, and a dry run, leave it alone
			runMediaClean(t, mgr, file.VerifyOptions{Paths: []string{filepath.Join(library, "other")}, PurgeMissing: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)
			runMediaClean(t, mgr, file.VerifyOptions{DryRun: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)

			runMediaClean(t, mgr, file.VerifyOptions{Paths: []string{library}, PurgeMissing: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 0, 1)
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, got, "the file itself stays")

			runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 0, 1)
		})
	}
}

// A video moved to the audio extensions and scanned again is reported as an
// audio file from then on, and Clean must still remove the scene made from it.
func TestCleanRemovesScenesOfVideoRescannedAsAudio(t *testing.T) {
	mgr, library := cleanMediaManager(t)
	mgr.Config.SetInterface(config.ImageExtensions, []string{"jpg"})
	path := filepath.Join(library, "clip.gif")
	require.NoError(t, os.WriteFile(path, cleanMediaGIF(t), 0644))
	f := scanCleanMedia(t, mgr, library, path)
	require.IsType(t, &models.VideoFile{}, f)
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 0)

	mgr.Config.SetInterface(config.VideoExtensions, []string{"mp4"})
	mgr.Config.SetInterface(config.AudioExtensions, []string{"gif"})
	f = scanCleanMedia(t, mgr, library, path)
	require.IsType(t, &models.AudioFile{}, f)
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 0)

	runMediaClean(t, mgr, file.VerifyOptions{DryRun: true})
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 0)
	runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
	requireCleanMediaCounts(t, mgr, f.Base().ID, 0, 0)
	require.NoError(t, mgr.Repository.WithReadTxn(context.Background(), func(ctx context.Context) error {
		audios, err := mgr.Repository.Audio.FindByFileID(ctx, f.Base().ID)
		require.NoError(t, err)
		require.Len(t, audios, 1, "the file stays as audio")
		return nil
	}))
}

// A scene with another file that is still a video keeps that file and loses
// only the one that isn't.
func TestCleanDetachesFileNoLongerVideoFromSharedScene(t *testing.T) {
	mgr, library := cleanMediaManager(t)
	inside := filepath.Join(library, "excluded", "clip.gif")
	outside := filepath.Join(library, "kept", "copy.gif")
	content := cleanMediaGIF(t)
	for _, path := range []string{inside, outside} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, content, 0644))
	}
	first := scanCleanMedia(t, mgr, library, inside)
	second := scanCleanMedia(t, mgr, library, outside)
	var sceneID int
	require.NoError(t, mgr.Repository.WithReadTxn(context.Background(), func(ctx context.Context) error {
		scenes, err := mgr.Repository.Scene.FindByFileID(ctx, first.Base().ID)
		if err == nil && len(scenes) == 1 {
			sceneID = scenes[0].ID
		}
		return err
	}))
	require.NotZero(t, sceneID, "both copies are one scene")

	mgr.Config.SetInterface(config.Stash, config.StashConfigs{{Path: library}, {Path: filepath.Dir(inside), ExcludeVideo: true}})
	runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
	requireCleanMediaCounts(t, mgr, first.Base().ID, 0, 1)
	requireCleanMediaCounts(t, mgr, second.Base().ID, 1, 1)

	var s *models.Scene
	require.NoError(t, mgr.Repository.WithReadTxn(context.Background(), func(ctx context.Context) error {
		var err error
		if s, err = mgr.Repository.Scene.Find(ctx, sceneID); err != nil || s == nil {
			return err
		}
		return s.LoadFiles(ctx, mgr.Repository.Scene)
	}))
	require.NotNil(t, s, "the scene stays")
	require.NotNil(t, s.PrimaryFileID)
	require.Equal(t, second.Base().ID, *s.PrimaryFileID)
	require.Len(t, s.Files.List(), 1)
}

// Clean removes stale images while retaining the shared video and its scene.
func TestCleanRemovesImagesOfFileNoLongerImage(t *testing.T) {
	for _, setting := range []string{"extension", "pattern", "library"} {
		t.Run(setting, func(t *testing.T) {
			mgr, library := cleanMediaManager(t)
			path := filepath.Join(library, "clip.gif")
			content := cleanMediaGIF(t)
			require.NoError(t, os.WriteFile(path, content, 0644))
			f := scanCleanMedia(t, mgr, library, path)
			require.IsType(t, &models.VideoFile{}, f)
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)

			runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)

			cfg := mgr.Config
			switch setting {
			case "extension":
				cfg.SetInterface(config.ImageExtensions, []string{"jpg"})
			case "pattern":
				cfg.SetInterface(config.ImageExclude, []string{`\.gif$`})
			case "library":
				cfg.SetInterface(config.Stash, config.StashConfigs{{Path: library, ExcludeImage: true}})
			}

			// Verification, a clean of another folder, and a dry run leave it alone
			runMediaClean(t, mgr, file.VerifyOptions{})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)
			runMediaClean(t, mgr, file.VerifyOptions{Paths: []string{filepath.Join(library, "other")}, PurgeMissing: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)
			runMediaClean(t, mgr, file.VerifyOptions{DryRun: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)

			runMediaClean(t, mgr, file.VerifyOptions{Paths: []string{library}, PurgeMissing: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 0)
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, got, "the file itself stays")

			runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
			requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 0)
		})
	}
}

// A shared image keeps its other copy and loses only the excluded file.
func TestCleanDetachesFileNoLongerImageFromSharedImage(t *testing.T) {
	mgr, library := cleanMediaManager(t)
	inside := filepath.Join(library, "excluded", "clip.gif")
	outside := filepath.Join(library, "kept", "copy.gif")
	content := cleanMediaGIF(t)
	for _, path := range []string{inside, outside} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, content, 0644))
	}
	first := scanCleanMedia(t, mgr, library, inside)
	second := scanCleanMedia(t, mgr, library, outside)
	var imageID int
	require.NoError(t, mgr.Repository.WithReadTxn(context.Background(), func(ctx context.Context) error {
		images, err := mgr.Repository.Image.FindByFileID(ctx, first.Base().ID)
		if err == nil && len(images) == 1 {
			imageID = images[0].ID
		}
		return err
	}))
	require.NotZero(t, imageID, "both copies are one image")

	mgr.Config.SetInterface(config.Stash, config.StashConfigs{{Path: library}, {Path: filepath.Dir(inside), ExcludeImage: true}})
	runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
	requireCleanMediaCounts(t, mgr, first.Base().ID, 1, 0)
	requireCleanMediaCounts(t, mgr, second.Base().ID, 1, 1)

	var s *models.Image
	require.NoError(t, mgr.Repository.WithReadTxn(context.Background(), func(ctx context.Context) error {
		var err error
		if s, err = mgr.Repository.Image.Find(ctx, imageID); err != nil || s == nil {
			return err
		}
		return s.LoadFiles(ctx, mgr.Repository.Image)
	}))
	require.NotNil(t, s, "the image stays")
	require.NotNil(t, s.PrimaryFileID)
	require.Equal(t, second.Base().ID, *s.PrimaryFileID)
	require.Len(t, s.Files.List(), 1)
}

func TestCleanRemovesImagesOfVideoRescannedAsAudio(t *testing.T) {
	mgr, library := cleanMediaManager(t)
	path := filepath.Join(library, "clip.gif")
	require.NoError(t, os.WriteFile(path, cleanMediaGIF(t), 0644))
	f := scanCleanMedia(t, mgr, library, path)
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)

	mgr.Config.SetInterface(config.VideoExtensions, []string{"mp4"})
	mgr.Config.SetInterface(config.ImageExtensions, []string{"jpg"})
	mgr.Config.SetInterface(config.AudioExtensions, []string{"gif"})
	f = scanCleanMedia(t, mgr, library, path)
	require.IsType(t, &models.AudioFile{}, f)
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)

	runMediaClean(t, mgr, file.VerifyOptions{DryRun: true})
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 1)
	runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
	requireCleanMediaCounts(t, mgr, f.Base().ID, 0, 0)
	require.NoError(t, mgr.Repository.WithReadTxn(context.Background(), func(ctx context.Context) error {
		audios, err := mgr.Repository.Audio.FindByFileID(ctx, f.Base().ID)
		require.NoError(t, err)
		require.Len(t, audios, 1, "the file stays as audio")
		return nil
	}))
}

func TestCleanKeepsVideoWithoutImage(t *testing.T) {
	mgr, library := cleanMediaManager(t)
	mgr.Config.SetInterface(config.ImageExtensions, []string{"jpg"})
	path := filepath.Join(library, "clip.gif")
	require.NoError(t, os.WriteFile(path, cleanMediaGIF(t), 0644))
	f := scanCleanMedia(t, mgr, library, path)
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 0)

	runMediaClean(t, mgr, file.VerifyOptions{DryRun: true})
	runMediaClean(t, mgr, file.VerifyOptions{PurgeMissing: true})
	requireCleanMediaCounts(t, mgr, f.Base().ID, 1, 0)
}
