package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_GetMaxSessionAge(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value interface{}
		want  int
	}{
		{"default", nil, DefaultMaxSessionAge},
		{"custom duration", 1800, 1800},
		{"numeric string", "7200", 7200},
		{"session cookie", 0, 0},
		{"negative legacy value", -1, -1},
		{"below limit", MaxSupportedSessionAge - 1, MaxSupportedSessionAge - 1},
		{"at limit", MaxSupportedSessionAge, MaxSupportedSessionAge},
		{"above limit", MaxSupportedSessionAge + 1, MaxSupportedSessionAge},
		{"large integer", int64(9223372036854775807), MaxSupportedSessionAge},
		{"reported value", 8.999999969665667e+61, MaxSupportedSessionAge},
		{"large numeric string", "8.999999969665667e+61", MaxSupportedSessionAge},
		{"beyond float range", "1e1000", MaxSupportedSessionAge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := InitializeEmpty()
			if tt.value != nil {
				cfg.SetInterface(MaxSessionAge, tt.value)
			}
			assert.Equal(t, tt.want, cfg.GetMaxSessionAge())
		})
	}
}
