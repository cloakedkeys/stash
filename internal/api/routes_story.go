package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/story"
)

type storyRoutes struct {
	routes
	storyFinder models.StoryGetter
	fileGetter  models.FileGetter
}

func (rs storyRoutes) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/{storyId}", func(r chi.Router) {
		r.Use(rs.StoryCtx)

		r.Get("/text", rs.Text)
	})

	return r
}

// Text serves the story's file as plain text, whatever its name, so that a
// file containing HTML or script is never run by the browser.
func (rs storyRoutes) Text(w http.ResponseWriter, r *http.Request) {
	story := r.Context().Value(storyKey).(*models.Story)

	f := story.Files.Primary()
	if f == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox")

	if err := serveStoryText(f.Base(), w, r); err != nil {
		if !errors.Is(err, context.Canceled) {
			logger.Warnf("error serving text of story %d: %v", story.ID, err)
		}
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}
}

// maxDecodedStorySize is the largest story file that is read whole to turn
// its text into UTF-8. A bigger file is sent as it is.
const maxDecodedStorySize = 64 * 1024 * 1024

// serveStoryText sends the file's text as UTF-8, whatever encoding it was
// saved in.
func serveStoryText(f *models.BaseFile, w http.ResponseWriter, r *http.Request) error {
	if f.Size > maxDecodedStorySize {
		return f.Serve(&file.OsFS{}, w, r)
	}

	reader, err := f.Open(&file.OsFS{})
	if err != nil {
		return err
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return err
	}

	if r.URL.Query().Has("t") {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`filename="%s"`, f.Basename))

	http.ServeContent(w, r, f.Basename, f.ModTime, bytes.NewReader(story.DecodeText(data)))
	return nil
}

func (rs storyRoutes) StoryCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storyID, err := strconv.Atoi(chi.URLParam(r, "storyId"))
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		// the store leaves out a story an account's library limit hides, so it
		// gets the same answer as one that doesn't exist
		var story *models.Story
		_ = rs.withReadTxn(r, func(ctx context.Context) error {
			story, _ = rs.storyFinder.Find(ctx, storyID)

			if story != nil {
				if err := story.LoadPrimaryFile(ctx, rs.fileGetter); err != nil {
					if !errors.Is(err, context.Canceled) {
						logger.Errorf("error loading primary file for story %d: %v", storyID, err)
					}
					story = nil
				}
			}

			return nil
		})
		if story == nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		ctx := context.WithValue(r.Context(), storyKey, story)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
