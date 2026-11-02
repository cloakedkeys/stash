package generate

import (
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
)

func TestPreviewVideoFilterSquarePixels(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
		ratio         float64
		codec         ffmpeg.VideoCodec
		fullhw        bool
		want          ffmpeg.VideoFilter
	}{
		{"square to widescreen", 1080, 1080, 16.0 / 9, ffmpeg.VideoCodecLibX264, false,
			"scale=trunc(iw*sar/2)*2:ih,setsar=1,scale=-2:360"},
		{"narrow pixels", 720, 480, 8.0 / 9, ffmpeg.VideoCodecLibX264, false,
			"scale=trunc(iw*sar/2)*2:ih,setsar=1"},
		{"display portrait", 1080, 720, 0.5, ffmpeg.VideoCodecLibX264, false,
			"scale=trunc(iw*sar/2)*2:ih,setsar=1"},
		{"hardware upload follows normalization", 1080, 1080, 16.0 / 9, ffmpeg.VideoCodecV264, true,
			"scale=trunc(iw*sar/2)*2:ih,setsar=1,format=nv12,hwupload,scale_vaapi=-2:360"},
		{"fixed hardware sizes use display ratio", 1080, 1080, 16.0 / 9, ffmpeg.VideoCodecI264, true,
			"scale=trunc(iw*sar/2)*2:ih,setsar=1,hwupload=extra_hw_frames=64,format=qsv,scale_qsv=640:360"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := Generator{Encoder: &ffmpeg.FFMpeg{}, SampleAspectRatio: tt.ratio}
			got, fullhw := g.previewVideoFilter(tt.codec, tt.width, tt.height, 640, tt.fullhw)
			if got != tt.want || fullhw {
				t.Fatalf("previewVideoFilter() = %q, %v; want %q, false", got, fullhw, tt.want)
			}
		})
	}
}

func TestPreviewVideoFilterNormalCommandUnchanged(t *testing.T) {
	for _, ratio := range []float64{0, 1} {
		for _, codec := range []ffmpeg.VideoCodec{ffmpeg.VideoCodecLibX264, ffmpeg.VideoCodecV264, ffmpeg.VideoCodecI264} {
			for _, fullhw := range []bool{false, true} {
				for _, dims := range [][2]int{{1920, 1080}, {1080, 1920}, {320, 240}} {
					g := Generator{Encoder: &ffmpeg.FFMpeg{}, SampleAspectRatio: ratio}
					width, height := dims[0], dims[1]
					want := g.Encoder.HWMaxResFilter(codec, width, height, ffmpeg.ScaledShortSide(width, height, 640), fullhw)
					got, gotHW := g.previewVideoFilter(codec, width, height, 640, fullhw)
					if got != want || gotHW != fullhw {
						t.Fatalf("normal filter = %q, %v; want %q, %v", got, gotHW, want, fullhw)
					}
				}
			}
		}
	}
}
