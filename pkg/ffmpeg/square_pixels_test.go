package ffmpeg

import "testing"

func TestSampleAspectRatio(t *testing.T) {
	for _, tt := range []struct {
		ratio string
		want  float64
	}{
		{"", 1}, {"N/A", 1}, {"0:1", 1}, {"1:0", 1},
		{"-1:2", 1}, {"bad:2", 1}, {"1:1", 1}, {"2:2", 1},
		{"16:9", 16.0 / 9}, {"8:9", 8.0 / 9},
	} {
		t.Run(tt.ratio, func(t *testing.T) {
			v := VideoFile{VideoStream: &FFProbeStream{SampleAspectRatio: tt.ratio}}
			if got := v.SampleAspectRatio(); got != tt.want {
				t.Fatalf("SampleAspectRatio() = %v, want %v", got, tt.want)
			}
		})
	}
	v := VideoFile{}
	if got := v.SampleAspectRatio(); got != 1 {
		t.Fatalf("missing stream ratio = %v, want 1", got)
	}
	v.VideoStream = &FFProbeStream{SampleAspectRatio: "16:9"}
	v.VideoStream.Tags.Rotate = "90"
	if got := v.SampleAspectRatio(); got != 9.0/16 {
		t.Fatalf("rotated ratio = %v, want 9/16", got)
	}
}

func TestSquarePixels(t *testing.T) {
	for _, tt := range []struct {
		ratio float64
		want  VideoFilter
	}{
		{0, "select=eq(n\\,3)"},
		{1, "select=eq(n\\,3)"},
		{16.0 / 9, "select=eq(n\\,3),scale=trunc(iw*sar/2)*2:ih,setsar=1"},
		{8.0 / 9, "select=eq(n\\,3),scale=trunc(iw*sar/2)*2:ih,setsar=1"},
	} {
		filter := VideoFilter("").Select(3).SquarePixels(tt.ratio)
		if filter != tt.want {
			t.Errorf("SquarePixels(%v) = %q, want %q", tt.ratio, filter, tt.want)
		}
	}
}

func TestSquarePixelDimensions(t *testing.T) {
	for _, tt := range []struct {
		width, height int
		ratio         float64
		wantWidth     int
	}{
		{1080, 1080, 16.0 / 9, 1920},
		{720, 480, 8.0 / 9, 640},
		{720, 1080, 2, 1440},  // stored portrait becomes display landscape
		{1080, 720, 0.5, 540}, // stored landscape becomes display portrait
		{721, 480, 1, 721},    // square pixels retain odd stored dimensions
		{721, 480, 0, 721},
		{721, 480, 8.0 / 9, 640},
	} {
		w, h := SquarePixelDimensions(tt.width, tt.height, tt.ratio)
		if w != tt.wantWidth || h != tt.height {
			t.Errorf("SquarePixelDimensions(%d, %d, %v) = %d, %d; want %d, %d", tt.width, tt.height, tt.ratio, w, h, tt.wantWidth, tt.height)
		}
	}
}
