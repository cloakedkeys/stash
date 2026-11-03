package manager

import (
	"context"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// cleanScenesOfKeptFile removes the scenes made from a video file that Clean
// keeps, when the settings no longer treat it as a video. A .webm moved from
// the video to the image extensions is kept as an image, so Clean doesn't
// remove the file, and without this its scene stayed (#1533).
func (f *verifyFilter) cleanScenesOfKeptFile(dryRun bool) func(context.Context, models.File) error {
	return func(ctx context.Context, kept models.File) error {
		// a video moved to the audio extensions and scanned again is read
		// back as an audio file, but keeps its scenes
		switch kept.(type) {
		case *models.VideoFile, *models.AudioFile:
		default:
			return nil
		}
		path := kept.Base().Path
		stash := f.stashPaths.GetStashFromPath(path)
		if stash == nil {
			return nil
		}
		if useAsVideo(path) && !f.shouldCleanVideoFile(path, stash) {
			return nil
		}

		mgr := GetInstance()
		// most audio files never had a scene, so look before writing
		var scenes int
		if err := mgr.Repository.WithReadTxn(ctx, func(ctx context.Context) error {
			var err error
			scenes, err = mgr.Repository.Scene.CountByFileID(ctx, kept.Base().ID)
			return err
		}); err != nil || scenes == 0 {
			return err
		}

		if dryRun {
			logger.Infof("%q is no longer used as a video. Would remove the scenes made from it.", path)
			return nil
		}
		return mgr.Repository.WithTxn(ctx, func(ctx context.Context) error {
			deleter := file.NewDeleterWithTrash(mgr.Config.GetDeleteTrashPath())
			deleter.RegisterHooks(ctx)
			h := &purgeHandler{keepFile: true}
			return h.handleRelatedScenes(ctx, deleter, kept.Base().ID)
		})
	}
}
