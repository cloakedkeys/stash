package api

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestSceneStreamSupportsHead(t *testing.T) {
	router := (sceneRoutes{}).Routes()
	if !router.Match(chi.NewRouteContext(), http.MethodHead, "/1/stream") {
		t.Fatal("direct scene stream does not support a HEAD availability check")
	}
}
