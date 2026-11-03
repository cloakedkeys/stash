package scraper

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type imageUserAgentConfig struct {
	mockGlobalConfig
	userAgent string
}

func (c imageUserAgentConfig) GetScraperUserAgent() string {
	return c.userAgent
}

func TestRemoteUserAgent(t *testing.T) {
	for _, userAgent := range []string{"", "ScraperTest/1.0"} {
		for _, override := range []bool{false, true} {
			cfg := imageUserAgentConfig{userAgent: userAgent}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				want := userAgent
				if override {
					want = "DriverTest/1.0"
				}
				if got := r.UserAgent(); (want != "" && got != want) || (want == "" && !strings.HasPrefix(got, "Mozilla/")) {
					t.Errorf("User-Agent = %q, want configured %q or browser default", got, want)
					w.WriteHeader(http.StatusTeapot)
					return
				}
				_, _ = w.Write([]byte("<svg/>"))
			}))

			getter := imageGetter{client: server.Client(), globalConfig: cfg}
			def := Definition{}
			if override {
				getter.requestModifier = func(req *http.Request) {
					req.Header.Set("User-Agent", "DriverTest/1.0")
				}
				def.DriverOptions = &scraperDriverOptions{Headers: []*header{{Key: "User-Agent", Value: "DriverTest/1.0"}}}
			}
			if _, err := getter.getImage(context.Background(), server.URL); err != nil {
				t.Errorf("getImage: %v", err)
			}
			reader, err := loadURL(context.Background(), server.URL, server.Client(), def, cfg, false)
			if err != nil {
				t.Errorf("loadURL: %v", err)
			} else if data, err := io.ReadAll(reader); err != nil || string(data) != "<svg/>" {
				t.Errorf("loadURL data = %q, error = %v", data, err)
			}
			server.Close()
		}
	}
}
