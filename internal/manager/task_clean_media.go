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
		if _, ok := kept.(*models.VideoFile); !ok {
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
