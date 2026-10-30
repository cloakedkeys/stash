package pkg

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

type testSourcePathGetter struct{}

func (testSourcePathGetter) GetAllSourcePaths() []string        { return []string{"src"} }
func (testSourcePathGetter) GetSourcePath(srcURL string) string { return "src" }

// writeTestSource writes a local package source containing a single package
// "a" which requires the given package IDs, serves it over HTTP and returns
// the index URL.
func writeTestSource(t *testing.T, requires ...string) string {
	t.Helper()
	dir := t.TempDir()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("a.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("name: a\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.zip"), buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	index := fmt.Sprintf("- id: a\n  name: a\n  version: \"1\"\n  date: 2024-01-01 00:00:00\n  path: a.zip\n  sha256: %x\n", sha256.Sum256(buf.Bytes()))
	if len(requires) > 0 {
		index += "  requires:\n"
		for _, r := range requires {
			index += "    - " + r + "\n"
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "index.yml"), []byte(index), 0644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	return srv.URL + "/index.yml"
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		Local:             &Store{BaseDir: t.TempDir(), ManifestFile: ManifestFile},
		PackagePathGetter: testSourcePathGetter{},
		Client:            http.DefaultClient,
	}
}

func TestManager_Install_MissingRequirement(t *testing.T) {
	source := writeTestSource(t, "missing")
	m := newTestManager(t)

	err := m.Install(context.Background(), models.PackageSpecInput{ID: "a", SourceURL: source})
	if err == nil {
		t.Fatal("expected an error for a requirement missing from the source, got nil")
	}
	if !strings.Contains(err.Error(), "package missing not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_Install_MissingPackage(t *testing.T) {
	source := writeTestSource(t)
	m := newTestManager(t)

	err := m.Install(context.Background(), models.PackageSpecInput{ID: "nope", SourceURL: source})
	if err == nil {
		t.Fatal("expected an error for a package missing from the source, got nil")
	}
}

func TestManager_Install_NoRequirements(t *testing.T) {
	source := writeTestSource(t)
	m := newTestManager(t)

	if err := m.Install(context.Background(), models.PackageSpecInput{ID: "a", SourceURL: source}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.Local.BaseDir, "src", "a", "a.yml")); err != nil {
		t.Errorf("expected installed package file: %v", err)
	}
}
