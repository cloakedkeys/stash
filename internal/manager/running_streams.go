package manager

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/internal/static"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stashapp/stash/pkg/utils"
)

func KillRunningStreams(scene *models.Scene, fileNamingAlgo models.HashAlgorithm) {
	instance.ReadLockManager.Cancel(scene.Path)

	sceneHash := scene.GetHash(fileNamingAlgo)

	if sceneHash == "" {
		return
	}

	transcodePath := GetInstance().Paths.Scene.GetTranscodePath(sceneHash)
	instance.ReadLockManager.Cancel(transcodePath)
}

func KillRunningStreamsAudio(audio *models.Audio, fileNamingAlgo models.HashAlgorithm) {
	instance.ReadLockManager.Cancel(audio.Path)
}

type SceneCoverGetter interface {
	GetCover(ctx context.Context, sceneID int) ([]byte, error)
}

type SceneServer struct {
	TxnManager       txn.Manager
	SceneCoverGetter SceneCoverGetter
}

func (s *SceneServer) StreamSceneDirect(scene *models.Scene, w http.ResponseWriter, r *http.Request) {
	// #3526 - return 404 if the scene does not have any files
	if scene.Path == "" {
		http.Error(w, http.StatusText(404), 404)
		return
	}

	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())

	fp := GetInstance().Paths.Scene.GetStreamPath(scene.Path, sceneHash)
	streamRequestCtx := ffmpeg.NewStreamRequestContext(w, r)

	// #2579 - hijacking and closing the connection here causes video playback to fail in Safari
	// We trust that the request context will be closed, so we don't need to call Cancel on the
	// returned context here.
	_ = GetInstance().ReadLockManager.ReadLock(streamRequestCtx, fp)
	_, filename := filepath.Split(fp)
	contentDisposition := mime.FormatMediaType("inline", map[string]string{"filename": filename})
	w.Header().Set("Content-Disposition", contentDisposition)
	http.ServeFile(w, r, fp)
}

func (s *SceneServer) ServeScreenshot(scene *models.Scene, w http.ResponseWriter, r *http.Request) {
	var cover []byte
	readTxnErr := txn.WithReadTxn(r.Context(), s.TxnManager, func(ctx context.Context) error {
		var err error
		cover, err = s.SceneCoverGetter.GetCover(ctx, scene.ID)
		return err
	})
	if errors.Is(readTxnErr, context.Canceled) {
		return
	}
	if readTxnErr != nil {
		logger.Warnf("read transaction error on fetch screenshot: %v", readTxnErr)
	}

	if cover == nil {
		// fallback to legacy image if present
		if scene.Path != "" {
			sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
			filepath := GetInstance().Paths.Scene.GetLegacyScreenshotPath(sceneHash)

			// fall back to the scene image blob if the file isn't present
			screenshotExists, _ := fsutil.FileExists(filepath)
			if screenshotExists && r.URL.Query().Get("thumbnail") == "true" {
				cover, _ = os.ReadFile(filepath)
			} else if screenshotExists {
				if r.URL.Query().Has("t") {
					w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				http.ServeFile(w, r, filepath)
				return
			}
		}

		// fallback to default cover if none found
		if cover == nil {
			cover = static.ReadAll(static.DefaultSceneImage)
		}
	}

	if r.URL.Query().Get("thumbnail") == "true" {
		cover = sceneCoverThumbnail(cover)
	}
	utils.ServeImage(w, r, cover)
}

// Keep the stored cover intact for the player, editing and exports. Unsupported
// formats (including SVG) are served unchanged.
func sceneCoverThumbnail(cover []byte) []byte {
	config, _, err := image.DecodeConfig(bytes.NewReader(cover))
	if err != nil || (config.Width <= 640 && config.Height <= 640) {
		return cover
	}

	img, err := imaging.Decode(bytes.NewReader(cover), imaging.AutoOrientation(true))
	if err != nil {
		return cover
	}

	thumb := imaging.Fit(img, 640, 640, imaging.Lanczos)
	var data bytes.Buffer
	if thumb.Opaque() {
		err = imaging.Encode(&data, thumb, imaging.JPEG, imaging.JPEGQuality(75))
	} else {
		err = png.Encode(&data, thumb)
	}
	if err != nil {
		return cover
	}
	return data.Bytes()
}

type AudioCoverGetter interface {
	GetCover(ctx context.Context, audioID int) ([]byte, error)
}

type AudioServer struct {
	TxnManager       txn.Manager
	AudioCoverGetter AudioCoverGetter
}

func (s *AudioServer) StreamAudioDirect(audio *models.Audio, w http.ResponseWriter, r *http.Request) {
	// #3526 - return 404 if the audio does not have any files
	if audio.Path == "" {
		http.Error(w, http.StatusText(404), 404)
		return
	}

	// audio is always served directly from the original file
	fp := audio.Path
	streamRequestCtx := ffmpeg.NewStreamRequestContext(w, r)

	// #2579 - hijacking and closing the connection here causes video playback to fail in Safari
	// We trust that the request context will be closed, so we don't need to call Cancel on the
	// returned context here.
	_ = GetInstance().ReadLockManager.ReadLock(streamRequestCtx, fp)
	_, filename := filepath.Split(fp)
	contentDisposition := mime.FormatMediaType("inline", map[string]string{"filename": filename})
	w.Header().Set("Content-Disposition", contentDisposition)
	http.ServeFile(w, r, fp)
}

// ServeScreenshot serves the audio's cover image. Covers are uploaded by hand or
// taken from the file's embedded art when it is scanned. Nothing is generated,
// so an audio without a cover falls back to the default image.
func (s *AudioServer) ServeScreenshot(audio *models.Audio, w http.ResponseWriter, r *http.Request) {
	var cover []byte
	readTxnErr := txn.WithReadTxn(r.Context(), s.TxnManager, func(ctx context.Context) error {
		var err error
		cover, err = s.AudioCoverGetter.GetCover(ctx, audio.ID)
		return err
	})
	if errors.Is(readTxnErr, context.Canceled) {
		return
	}
	if readTxnErr != nil {
		logger.Warnf("read transaction error on fetch audio screenshot: %v", readTxnErr)
	}

	if cover == nil {
		cover = static.ReadAll(static.DefaultAudioImage)
	}

	utils.ServeImage(w, r, cover)
}
