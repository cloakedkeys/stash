package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCDPPath(t *testing.T) {
	ctx := context.Background()

	notCDP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Browser": "not chrome"}`))
	}))
	defer notCDP.Close()

	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()

	// reports a websocket address that nothing listens on
	deadWS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl": "ws://127.0.0.1:1/devtools/browser/x"}`))
	}))
	defer deadWS.Close()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty path", "", false},
		{"blank path", "   ", false},
		{"existing local executable", exe, false},
		{"missing local executable", filepath.Join(t.TempDir(), "no-chrome.exe"), true},
		{"http without webSocketDebuggerUrl", notCDP.URL + "/json/version", true},
		{"http error status", notFound.URL + "/json/version", true},
		{"http with unreachable websocket", deadWS.URL + "/json/version", true},
		{"unreachable ws", "ws://127.0.0.1:1/devtools/browser/x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := ValidateCDPPath(ctx, tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCDPPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if err == nil && status == "" {
				t.Errorf("ValidateCDPPath(%q) returned an empty status", tt.path)
			}
		})
	}
}
