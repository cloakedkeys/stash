package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2146: turning the guest account on or off used to wait forever for the
// config lock, and every later config read waited behind it.
func TestSetGuestUserEnabledFinishes(t *testing.T) {
	cfg := &Config{main: koanf.New("."), overrides: koanf.New(".")}
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg.SetConfigFile(path)

	for _, enabled := range []bool{true, false, true} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			cfg.SetGuestUserEnabled(enabled)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("SetGuestUserEnabled(%v) didn't finish", enabled)
		}

		assert.Equal(t, enabled, cfg.GetGuestUserEnabled())
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		want := "guest_account_enabled: false"
		if enabled {
			want = "guest_account_enabled: true"
		}
		assert.True(t, strings.Contains(string(data), want), "config file:\n%s", data)
	}
}

// Turning the guest account on or off while other settings are read must not
// race with them. Run with -race.
func TestSetGuestUserEnabledWithReaders(t *testing.T) {
	cfg := &Config{main: koanf.New("."), overrides: koanf.New(".")}
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = cfg.GetGuestUserEnabled()
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for n := 0; n < 20; n++ {
			cfg.SetGuestUserEnabled(n%2 == 0)
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("SetGuestUserEnabled didn't finish with readers running")
	}
	close(stop)
	wg.Wait()
	assert.False(t, cfg.GetGuestUserEnabled())
}
