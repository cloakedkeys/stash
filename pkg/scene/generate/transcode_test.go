package generate

import (
	"reflect"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
)

type transcodeTestConfig struct{}

func (transcodeTestConfig) GetGenerationThreads() int { return 4 }

func (transcodeTestConfig) GetTranscodeInputArgs() []string         { return nil }
func (transcodeTestConfig) GetTranscodeOutputArgs() []string        { return nil }
func (transcodeTestConfig) GetGenerationHardwareAcceleration() bool { return true }

// videoFilterArg returns the value of -vf, or "" if there is none.
func videoFilterArg(args ffmpeg.Args) string {
	for i, arg := range args {
		if arg == "-vf" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestTranscodeVideoArgs(t *testing.T) {
	const width, height = 1920, 1080
	for _, tt := range []struct {
		name       string
		codec      ffmpeg.VideoCodec
		maxSize    int
		wantFilter string
		wantInput  ffmpeg.Args
	}{
		{"software original size", ffmpeg.VideoCodecLibX264, 0, "", nil},
		{"software scaled", ffmpeg.VideoCodecLibX264, 720, "scale=-2:720", nil},
		// VAAPI only takes frames uploaded to the GPU, even without scaling
		{"vaapi original size", ffmpeg.VideoCodecV264, 0, "format=nv12,hwupload",
			ffmpeg.Args{"-vaapi_device", "/dev/dri/renderD128"}},
		{"vaapi scaled", ffmpeg.VideoCodecV264, 720, "format=nv12,hwupload,scale_vaapi=-2:720",
			ffmpeg.Args{"-vaapi_device", "/dev/dri/renderD128"}},
		// AMF needs 8-bit frames
		{"amf original size", ffmpeg.VideoCodecA264, 0, "format=nv12", nil},
		{"amf scaled", ffmpeg.VideoCodecA264, 720, "scale=-2:720,format=nv12", nil},
		// NVENC gets 8-bit 4:2:0 frames uploaded, as for previews
		{"nvenc original size", ffmpeg.VideoCodecN264, 0, "format=nv12,hwupload_cuda",
			ffmpeg.Args{"-hwaccel_device", "0"}},
		{"nvenc scaled", ffmpeg.VideoCodecN264, 720, "format=nv12,hwupload_cuda,scale_cuda=-2:720",
			ffmpeg.Args{"-hwaccel_device", "0"}},
		// Quick Sync keeps no filter at original size until #2165 is fixed
		{"qsv original size", ffmpeg.VideoCodecI264, 0, "",
			ffmpeg.Args{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}},
		{"qsv scaled", ffmpeg.VideoCodecI264, 720, "format=nv12,hwupload=extra_hw_frames=64,format=qsv,scale_qsv=1280:720",
			ffmpeg.Args{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STASH_HW_DRI_DEVICE", "")
			g := Generator{Encoder: &ffmpeg.FFMpeg{}, FFMpegConfig: transcodeTestConfig{}}
			videoArgs, inputArgs := g.transcodeVideoArgs(tt.codec, false, width, height, TranscodeOptions{MaxSize: tt.maxSize})

			if got := videoFilterArg(videoArgs); got != tt.wantFilter {
				t.Errorf("video filter = %q, want %q (args %v)", got, tt.wantFilter, videoArgs)
			}
			if len(inputArgs) != 0 || len(tt.wantInput) != 0 {
				if !reflect.DeepEqual(inputArgs, tt.wantInput) {
					t.Errorf("input args = %v, want %v", inputArgs, tt.wantInput)
				}
			}
		})
	}
}

func TestTranscodeVideoArgsAMFQuality(t *testing.T) {
	g := Generator{Encoder: &ffmpeg.FFMpeg{}, FFMpegConfig: transcodeTestConfig{}}
	videoArgs, _ := g.transcodeVideoArgs(ffmpeg.VideoCodecA264, false, 1920, 1080, TranscodeOptions{})
	want := ffmpeg.Args{"-quality", "quality", "-vf", "format=nv12"}
	if len(videoArgs) < len(want) || !reflect.DeepEqual(videoArgs[:len(want)], want) {
		t.Errorf("video args = %v, want prefix %v", videoArgs, want)
	}
}
