package python

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolvePath(t *testing.T) {
	const alias3 = `C:\Users\test\AppData\Local\Microsoft\WindowsApps\python3.exe`
	const alias = `C:\Users\test\AppData\Local\Microsoft\WindowsApps\python.exe`
	const real3 = `C:\Python313\python3.exe`
	const real = `C:\Python313\python.exe`

	tests := []struct {
		name  string
		goos  string
		paths map[string]string
		want  string
		calls []string
	}{
		{"prefer python3", "windows", map[string]string{"python3": real3, "python": real}, real3, []string{"python3"}},
		{"fall back to python", "windows", map[string]string{"python": real}, real, []string{"python3", "python"}},
		{"skip python3 alias", "windows", map[string]string{"python3": alias3, "python": real}, real, []string{"python3", "python"}},
		{"case insensitive folder", "windows", map[string]string{"python3": `C:\Users\test\AppData\Local\Microsoft\WINDOWSAPPS\python3.exe`, "python": real}, real, []string{"python3", "python"}},
		{"forward slashes", "windows", map[string]string{"python3": "C:/Users/test/AppData/Local/Microsoft/WindowsApps/python3.exe", "python": real}, real, []string{"python3", "python"}},
		{"folder boundary", "windows", map[string]string{"python3": `C:\WindowsAppsBackup\python3.exe`, "python": real}, `C:\WindowsAppsBackup\python3.exe`, []string{"python3"}},
		{"Store python3 only", "windows", map[string]string{"python3": alias3}, alias3, []string{"python3", "python"}},
		{"both Store aliases", "windows", map[string]string{"python3": alias3, "python": alias}, alias3, []string{"python3", "python"}},
		{"Store python only", "windows", map[string]string{"python": alias}, alias, []string{"python3", "python"}},
		{"not found", "windows", nil, "", []string{"python3", "python"}},
		{"Linux unchanged", "linux", map[string]string{"python3": alias3, "python": real}, alias3, []string{"python3"}},
		{"macOS unchanged", "darwin", map[string]string{"python3": alias3, "python": real}, alias3, []string{"python3"}},
		{"Android unchanged", "android", map[string]string{"python3": alias3, "python": real}, alias3, []string{"python3"}},
		{"Linux fallback unchanged", "linux", map[string]string{"python": "/usr/bin/python"}, "/usr/bin/python", []string{"python3", "python"}},
		{"Linux not found unchanged", "linux", nil, "", []string{"python3", "python"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			got, err := resolvePath(tt.goos, func(name string) (string, error) {
				calls = append(calls, name)
				if path, ok := tt.paths[name]; ok {
					return path, nil
				}
				return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
			})
			if got != tt.want {
				t.Errorf("path = %q, want %q", got, tt.want)
			}
			if tt.want == "" {
				if !errors.Is(err, exec.ErrNotFound) {
					t.Errorf("error = %v, want exec.ErrNotFound", err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(calls, tt.calls) {
				t.Errorf("lookups = %v, want %v", calls, tt.calls)
			}
		})
	}
}

func TestResolveConfiguredPath(t *testing.T) {
	// Even a WindowsApps path must be honored when explicitly configured.
	dir := filepath.Join(t.TempDir(), "WindowsApps")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "python3.exe")
	if err := os.WriteFile(path, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	got, err := Resolve(path)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || string(*got) != path {
		t.Fatalf("Resolve(%q) = %v, want configured path", path, got)
	}
}
