package ffmpeg

import (
	"reflect"
	"testing"
)

func TestArgs_MovieTimescale(t *testing.T) {
	tests := []struct {
		name    string
		version Version
		want    []string
	}{
		// before 9 the muxer already uses 1000, and versions before 4.2 lack the option
		{"version not detected", Version{}, nil},
		{"8.1", Version{8, 1, 3}, nil},
		{"9.0", Version{9, 0, 2}, []string{"-movie_timescale", "1000"}},
		{"10.0", Version{10, 0, 0}, []string{"-movie_timescale", "1000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Args{}.MovieTimescale(tt.version)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual([]string(got), tt.want) {
				t.Errorf("MovieTimescale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArgs_VSync(t *testing.T) {
	tests := []struct {
		name    string
		method  VSyncMethod
		version Version
		want    []string
	}{
		{"version not detected", VSyncMethodPassthrough, Version{}, []string{"-vsync", "0"}},
		{"before 5.1", VSyncMethodPassthrough, Version{5, 0, 3}, []string{"-vsync", "0"}},
		{"5.1", VSyncMethodPassthrough, Version{5, 1, 0}, []string{"-fps_mode", "passthrough"}},
		{"9.0", VSyncMethodPassthrough, Version{9, 0, 2}, []string{"-fps_mode", "passthrough"}},
		{"cfr", VSyncMethodCFR, Version{9, 0, 0}, []string{"-fps_mode", "cfr"}},
		{"vfr", VSyncMethodVFR, Version{9, 0, 0}, []string{"-fps_mode", "vfr"}},
		{"auto", VSyncMethodAuto, Version{9, 0, 0}, []string{"-fps_mode", "auto"}},
		{"drop has no fps_mode", VSyncMethodDrop, Version{9, 0, 0}, []string{"-vsync", "drop"}},
		{"empty", "", Version{9, 0, 0}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Args{}.VSync(tt.method, tt.version)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual([]string(got), tt.want) {
				t.Errorf("VSync() = %v, want %v", got, tt.want)
			}
		})
	}
}
