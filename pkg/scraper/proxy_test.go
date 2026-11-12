package scraper

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

type proxyTestConfig struct {
	mockGlobalConfig
	proxy, chrome, python string
}

func (c proxyTestConfig) GetProxy() string          { return c.proxy }
func (c proxyTestConfig) GetScraperCDPPath() string { return c.chrome }
func (c proxyTestConfig) GetPythonPath() string     { return c.python }

// ProxyFromEnvironment caches the environment once per process. Run each scheme
// in a fresh process so neither the suite nor the user's proxy affects the test.
func TestScraperSOCKSProxy(t *testing.T) {
	scheme := os.Getenv("STASH_TEST_SOCKS_SCHEME")
	if scheme == "" {
		for _, scheme := range []string{"socks5", "socks5h"} {
			for _, targetScheme := range []string{"http", "https"} {
				t.Run(scheme+"/"+targetScheme, func(t *testing.T) {
					cmd := exec.Command(os.Args[0], "-test.run=^TestScraperSOCKSProxy$", "-test.v")
					cmd.Env = append(os.Environ(), "STASH_TEST_SOCKS_SCHEME="+scheme, "STASH_TEST_SOCKS_TARGET_SCHEME="+targetScheme)
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("%v\n%s", err, out)
					}
				})
			}
		}
		return
	}

	targetScheme := os.Getenv("STASH_TEST_SOCKS_TARGET_SCHEME")
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "scraped content")
	}))
	if targetScheme == "https" {
		target.StartTLS()
	} else {
		target.Start()
	}
	defer target.Close()
	targetAddr := strings.TrimPrefix(target.URL, targetScheme+"://")
	_, port, _ := net.SplitHostPort(targetAddr)
	// A non-loopback hostname avoids Go's intentional localhost proxy bypass.
	// The SOCKS server alone resolves this name to our loopback target.
	url := targetScheme + "://scraper.invalid:" + port + "/"
	proxy, requests := socksTestServer(t, targetAddr)
	proxyURL := scheme + "://" + proxy
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(key, proxyURL)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(key, "")
	}
	gc := proxyTestConfig{proxy: proxyURL}
	client := newClient(gc)
	defer client.CloseIdleConnections()
	for _, mode := range []string{"HTTP", "image", "Surf"} {
		t.Run(mode, func(t *testing.T) {
			client.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if mode == "image" {
				got, err := getImage(ctx, url, client, gc)
				if err != nil || got == nil || *got != "data:image/png;base64,c2NyYXBlZCBjb250ZW50" {
					t.Fatalf("getImage = %v, %v", got, err)
				}
			} else {
				def := Definition{}
				if mode == "Surf" {
					def.DriverOptions = &scraperDriverOptions{UseSurf: true}
				}
				r, err := loadURL(ctx, url, client, def, gc, false)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(r)
				if err != nil || string(body) != "scraped content" {
					t.Fatalf("body = %q, %v", body, err)
				}
			}
			select {
			case addr := <-requests:
				if addr != "scraper.invalid:"+port {
					t.Fatalf("SOCKS CONNECT = %q", addr)
				}
			case <-ctx.Done():
				t.Fatal("request did not pass through SOCKS")
			}
		})
	}
}

// A minimal, no-auth SOCKS5 CONNECT server. It only forwards the expected
// hostname to a fixed local target; it never accesses the outside network.
func socksTestServer(t *testing.T, target string) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	requests := make(chan string, 16)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
				header := make([]byte, 2)
				if _, err := io.ReadFull(conn, header); err != nil || header[0] != 5 {
					return
				}
				methods := make([]byte, int(header[1]))
				if _, err := io.ReadFull(conn, methods); err != nil || !slices.Contains(methods, byte(0)) {
					return
				}
				if _, err := conn.Write([]byte{5, 0}); err != nil {
					return
				}
				header = make([]byte, 5)
				if _, err := io.ReadFull(conn, header); err != nil || !slices.Equal(header[:4], []byte{5, 1, 0, 3}) {
					return
				}
				addr := make([]byte, int(header[4])+2)
				if _, err := io.ReadFull(conn, addr); err != nil {
					return
				}
				host := string(addr[:len(addr)-2])
				if host != "scraper.invalid" {
					return
				}
				requests <- net.JoinHostPort(host, fmt.Sprint(binary.BigEndian.Uint16(addr[len(addr)-2:])))
				upstream, err := net.DialTimeout("tcp", target, 5*time.Second)
				if err != nil {
					return
				}
				defer upstream.Close()
				if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
					return
				}
				done := make(chan struct{})
				go func() { _, _ = io.Copy(upstream, conn); close(done) }()
				_, _ = io.Copy(conn, upstream)
				_ = conn.Close()
				<-done
			}()
		}
	}()
	return listener.Addr().String(), requests
}

// The test binary doubles as Chrome, recording the real launch arguments before
// exiting. No installed browser or live browsing session is used.
func init() {
	if os.Getenv("STASH_TEST_SCRIPT_PROXY") == "1" {
		values := make(map[string]string)
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
			values[key] = os.Getenv(key)
		}
		if err := json.NewEncoder(os.Stdout).Encode(values); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if file := os.Getenv("STASH_TEST_CHROME_ARGS"); file != "" {
		data, err := json.Marshal(os.Args[1:])
		if err != nil || os.WriteFile(file, data, 0600) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
}

func TestCDPSOCKSProxyArguments(t *testing.T) {
	dir := t.TempDir()
	name := "headless_shell"
	if runtime.GOOS == "windows" {
		name = "chrome.exe"
	}
	binary, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer binary.Close()
	chrome := filepath.Join(dir, name)
	fakeChrome, err := os.OpenFile(chrome, os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(fakeChrome, binary)
	closeErr := fakeChrome.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Setenv("PATH", dir)
	for _, path := range []string{chrome, ""} {
		for _, scheme := range []string{"socks5", "socks5h"} {
			t.Run(fmt.Sprintf("path=%s/%s", path, scheme), func(t *testing.T) {
				if path == "" && runtime.GOOS == "darwin" {
					t.Skip("chromedp searches absolute application paths before PATH on macOS")
				}
				file := filepath.Join(t.TempDir(), "args.json")
				t.Setenv("STASH_TEST_CHROME_ARGS", file)
				gc := proxyTestConfig{chrome: path, proxy: scheme + "://127.0.0.1:1080"}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, err := urlFromCDP(ctx, "http://scraper.invalid", scraperDriverOptions{UseCDP: true}, gc, false)
				if err == nil {
					t.Fatal("fake Chrome should exit without starting CDP")
				}
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				var args []string
				if err := json.Unmarshal(data, &args); err != nil {
					t.Fatal(err)
				}
				if !slices.Contains(args, "--proxy-server=socks5://127.0.0.1:1080") {
					t.Fatalf("Chrome proxy argument missing: %v", args)
				}
			})
		}
	}
}

func TestScriptProxyEnvironment(t *testing.T) {
	t.Setenv("STASH_TEST_SCRIPT_PROXY", "1")
	want := map[string]string{
		"HTTP_PROXY":  "socks5://127.0.0.1:1080",
		"HTTPS_PROXY": "socks5://127.0.0.1:1080",
		"NO_PROXY":    "excluded.invalid",
	}
	for key, value := range want {
		t.Setenv(key, value)
	}
	for _, command := range []string{os.Args[0], "python"} {
		t.Run(command, func(t *testing.T) {
			s := scriptScraper{
				definition:   Definition{path: filepath.Join(t.TempDir(), "scraper.yml")},
				globalConfig: proxyTestConfig{python: os.Args[0]},
			}
			var got map[string]string
			if err := s.runScraperScript(context.Background(), []string{command}, "", &got); err != nil {
				t.Fatal(err)
			}
			for key, value := range want {
				if got[key] != value {
					t.Errorf("script %s = %q, want %q", key, got[key], value)
				}
			}
		})
	}
}
