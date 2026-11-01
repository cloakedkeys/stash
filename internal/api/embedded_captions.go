package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/scene/embedded"
	"github.com/stashapp/stash/pkg/utils"
)

func embeddedCache(sceneID int, f *models.VideoFile) embedded.Cache {
	m := manager.GetInstance()
	c := embedded.Cache{Dir: m.Paths.Scene.GetEmbeddedSubtitleDir(sceneID), Source: f.Path, FileID: fmt.Sprint(f.ID)}
	if m.FFMpeg != nil {
		c.FFMpeg = m.FFMpeg.Path()
	}
	if m.FFProbe != nil {
		c.FFProbe = m.FFProbe.Path()
	}
	return c
}

func (r *sceneResolver) EmbeddedCaptions(ctx context.Context, obj *models.Scene) ([]*embedded.Track, error) {
	f, err := r.getPrimaryFile(ctx, obj)
	if err != nil || f == nil {
		return nil, err
	}
	tracks, err := embeddedCache(obj.ID, f).Tracks(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]*embedded.Track, len(tracks))
	for i := range tracks {
		ret[i] = &tracks[i]
	}
	return ret, nil
}

// EmbeddedCaption is reached through the same SceneCtx and authentication
// middleware as external captions. SceneCtx has already closed its transaction.
func (rs sceneRoutes) EmbeddedCaption(w http.ResponseWriter, r *http.Request, rawIndex string) {
	index, err := strconv.Atoi(rawIndex)
	if err != nil || index < 0 {
		http.Error(w, "invalid stream index", http.StatusBadRequest)
		return
	}
	s := r.Context().Value(sceneKey).(*models.Scene)
	f := s.Files.Primary()
	if f == nil {
		http.NotFound(w, r)
		return
	}
	target, err := embeddedCache(s.ID, f).Extract(r.Context(), index)
	if errors.Is(err, embedded.ErrTrackNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			http.Error(w, "could not extract embedded caption", http.StatusInternalServerError)
		}
		return
	}
	// Always validate the current file version, including when a browser has a
	// previously cached URL for this stream index.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/vtt")
	utils.ServeStaticFile(w, r, target)
}
