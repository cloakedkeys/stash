package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/pkg/file/video"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPartSidecarsUnavailable(t *testing.T) {
	start, end := 3.25, 8.75
	for _, bounds := range []struct {
		name       string
		start, end *float64
	}{
		{"both", &start, &end}, {"start only", &start, nil}, {"end only", nil, &end}, {"whole", nil, nil},
	} {
		t.Run(bounds.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "source.mp4")
			require.NoError(t, os.WriteFile(video.GetFunscriptPath(file), []byte(`{"version":"1.0","actions":[{"at":0,"pos":0},{"at":1000,"pos":100}]}`), 0600))
			s := &models.Scene{ID: 1, Path: file, StartTime: bounds.start, EndTime: bounds.end}
			s.Files.SetPrimary(&models.VideoFile{BaseFile: &models.BaseFile{Path: file}})
			db := mocks.NewDatabase()
			repo := db.Repository()
			db.File.On("GetCaptions", mock.Anything, mock.Anything).Return([]*models.VideoCaption{}, nil).Maybe()
			rs := sceneRoutes{routes: routes{txnManager: repo.TxnManager}, captionFinder: db.File}
			handlers := map[string]http.HandlerFunc{
				"funscript": rs.Funscript, "interactive csv": rs.InteractiveCSV,
				"external caption": func(w http.ResponseWriter, r *http.Request) { rs.Caption(w, r, "eng", "srt") },
				"embedded caption": func(w http.ResponseWriter, r *http.Request) { rs.EmbeddedCaption(w, r, "-1") },
				"caption query":    rs.CaptionLang,
			}
			for name, handler := range handlers {
				t.Run(name, func(t *testing.T) {
					req := httptest.NewRequest("GET", "/scene/1/caption?stream=invalid", nil)
					req = req.WithContext(context.WithValue(req.Context(), sceneKey, s))
					w := httptest.NewRecorder()
					handler(w, req)
					want := 200
					if bounds.start != nil || bounds.end != nil {
						want = 404
					} else if name == "embedded caption" || name == "caption query" {
						want = 400
					}
					require.Equal(t, want, w.Code, w.Body.String())
				})
			}
		})
	}
}

func TestPartCaptionMetadataUnavailable(t *testing.T) {
	start := 3.25
	fileID := models.FileID(1)
	speedValue := 60
	s := &models.Scene{PrimaryFileID: &fileID}
	db := mocks.NewDatabase()
	r := newResolver(db).Scene()
	var ctx context.Context
	middleware := loaders.Middleware{Repository: db.Repository()}
	middleware.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		ctx = req.Context()
		loaders.From(ctx).FileByID.Prime(fileID, &models.VideoFile{
			BaseFile: &models.BaseFile{ID: fileID}, Interactive: true, InteractiveSpeed: &speedValue,
		})
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	interactive, err := r.Interactive(ctx, s)
	require.NoError(t, err)
	require.True(t, interactive)
	speed, err := r.InteractiveSpeed(ctx, s)
	require.NoError(t, err)
	require.Equal(t, &speedValue, speed)
	s.StartTime = &start
	interactive, err = r.Interactive(ctx, s)
	require.NoError(t, err)
	require.False(t, interactive)
	speed, err = r.InteractiveSpeed(ctx, s)
	require.NoError(t, err)
	require.Nil(t, speed)
	external, err := r.Captions(ctx, s)
	require.NoError(t, err)
	require.Empty(t, external)
	embedded, err := r.EmbeddedCaptions(ctx, s)
	require.NoError(t, err)
	require.Empty(t, embedded)
}
