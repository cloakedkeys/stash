package api

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestAudioRoutesExposeDirectStreamAndCover(t *testing.T) {
	router := (audioRoutes{}).Routes()
	for _, path := range []string{"/1/stream", "/1/screenshot"} {
		if !router.Match(chi.NewRouteContext(), http.MethodGet, path) {
			t.Fatalf("audio router does not expose GET %s", path)
		}
	}
}
