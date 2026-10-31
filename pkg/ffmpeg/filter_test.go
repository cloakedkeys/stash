package ffmpeg

import "testing"

func TestScaledShortSide(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		targetWidth   int
		want          int
	}{
		{"landscape", 1920, 1080, 640, 360},
		{"portrait", 1080, 1920, 640, 640},
		{"square", 1000, 1000, 640, 640},
		{"audio only", 0, 0, 640, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScaledShortSide(tt.width, tt.height, tt.targetWidth); got != tt.want {
				t.Errorf("ScaledShortSide(%d, %d, %d) = %d, want %d", tt.width, tt.height, tt.targetWidth, got, tt.want)
			}
		})
	}
}

func TestScaleMax(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		reqHeight     int
		want          VideoFilter
	}{
		{"landscape preview", 1920, 1080, ScaledShortSide(1920, 1080, 640), "scale=-2:360"},
		{"portrait preview", 1080, 1920, ScaledShortSide(1080, 1920, 640), "scale=640:-2"},
		{"landscape transcode", 1920, 1080, 720, "scale=-2:720"},
		{"portrait transcode", 1080, 1920, 720, "scale=720:-2"},
		{"no upscale", 640, 360, 720, ""},
		{"zero request", 1920, 1080, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f VideoFilter
			if got := f.ScaleMax(tt.width, tt.height, tt.reqHeight, 0, 0); got != tt.want {
				t.Errorf("ScaleMax(%d, %d, %d) = %q, want %q", tt.width, tt.height, tt.reqHeight, got, tt.want)
			}
		})
	}
}
