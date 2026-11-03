package stashbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchImageUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.UserAgent(); !strings.HasPrefix(got, "Mozilla/") {
			t.Errorf("User-Agent = %q, want browser default", got)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte("<svg/>"))
	}))
	defer server.Close()

	image, err := fetchImage(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if image == nil || *image != "data:image/svg+xml;base64,PHN2Zy8+" {
		t.Fatalf("image = %v, want downloaded SVG data URI", image)
	}
}
