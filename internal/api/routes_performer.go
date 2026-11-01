package api

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/utils"
)

type PerformerFinder interface {
	models.PerformerGetter
	GetImage(ctx context.Context, performerID int) ([]byte, error)
	GetAlternateImage(ctx context.Context, performerID int, checksum string) ([]byte, error)
}

type sfwConfig interface {
	GetSFWContentMode() bool
}

type performerRoutes struct {
	routes
	performerFinder PerformerFinder
	sfwConfig       sfwConfig
}

func (rs performerRoutes) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/{performerId}", func(r chi.Router) {
		r.Use(rs.PerformerCtx)
		r.Get("/image", rs.Image)
		r.Get("/image/{checksum}", rs.AlternateImage)
	})

	return r
}

func (rs performerRoutes) Image(w http.ResponseWriter, r *http.Request) {
	performer := r.Context().Value(performerKey).(*models.Performer)
	defaultParam := r.URL.Query().Get("default")

	var image []byte
	if defaultParam != "true" {
		readTxnErr := rs.withReadTxn(r, func(ctx context.Context) error {
			var err error
			image, err = rs.performerFinder.GetImage(ctx, performer.ID)
			return err
		})
		if errors.Is(readTxnErr, context.Canceled) {
			return
		}
		if readTxnErr != nil {
			logger.Warnf("read transaction error on fetch performer image: %v", readTxnErr)
		}
	}

	if len(image) == 0 {
		image = getDefaultPerformerImage(performer.Name, performer.Gender, rs.sfwConfig.GetSFWContentMode())
	}

	utils.ServeImage(w, r, image)
}

func (rs performerRoutes) PerformerCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		performerID, err := strconv.Atoi(chi.URLParam(r, "performerId"))
		if err != nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		var performer *models.Performer
		_ = rs.withReadTxn(r, func(ctx context.Context) error {
			var err error
			performer, err = rs.performerFinder.Find(ctx, performerID)
			return err
		})
		if performer == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		ctx := context.WithValue(r.Context(), performerKey, performer)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (rs performerRoutes) AlternateImage(w http.ResponseWriter, r *http.Request) {
	checksum := chi.URLParam(r, "checksum")
	decoded, err := hex.DecodeString(checksum)
	if err != nil || len(decoded) != 16 || checksum != hex.EncodeToString(decoded) {
		http.Error(w, "invalid image checksum", http.StatusBadRequest)
		return
	}
	p := r.Context().Value(performerKey).(*models.Performer)
	var data []byte
	err = rs.withReadTxn(r, func(ctx context.Context) error {
		var err error
		data, err = rs.performerFinder.GetAlternateImage(ctx, p.ID, checksum)
		return err
	})
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(data) == 0) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	// Match the immutable caching used for versioned images, including direct route requests.
	q := r.URL.Query()
	q.Set("t", checksum)
	r.URL.RawQuery = q.Encode()
	utils.ServeImage(w, r, data)
}
