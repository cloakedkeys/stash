package ffmpeg

import (
	"slices"
	"testing"
)

var allVideoCodecs = []VideoCodec{
	VideoCodecLibX264, VideoCodecLibWebP, VideoCodecBMP, VideoCodecMJpeg,
	VideoCodecVP9, VideoCodecVPX, VideoCodecLibX265, VideoCodecCopy,
	VideoCodecN264, VideoCodecN264H, VideoCodecI264, VideoCodecI264C,
	VideoCodecA264, VideoCodecM264, VideoCodecV264, VideoCodecR264,
	VideoCodecO264, VideoCodecIVP9, VideoCodecVVP9, VideoCodecVVPX,
	VideoCodecRK264,
}

// Without a stream specifier, ffmpeg warns that -profile and -b are ambiguous,
// and applies -q, -qscale, -global_quality and -level to the audio encoder too.
var unspecifiedOptions = []string{"-profile", "-b", "-q", "-qscale", "-global_quality", "-level"}

func checkStreamSpecified(t *testing.T, name string, args Args) {
	t.Helper()
	for _, arg := range args {
		if slices.Contains(unspecifiedOptions, arg) {
			t.Errorf("%s: %s has no stream specifier in %v", name, arg, args)
		}
	}
}

func TestCodecArgsUseStreamSpecifiers(t *testing.T) {
	for _, codec := range allVideoCodecs {
		checkStreamSpecified(t, codec.Name+" ExtraArgs", codec.ExtraArgs())
		checkStreamSpecified(t, codec.Name+" ExtraArgsHQ", codec.ExtraArgsHQ("slow", 21, 4))

		for _, format := range []StreamFormat{StreamTypeMP4, StreamTypeWEBM, StreamTypeMKV} {
			checkStreamSpecified(t, codec.Name+" "+format.MimeType, format.Args(codec, "", false))
		}
		for _, stream := range []*StreamType{StreamTypeHLS, StreamTypeHLSCopy, StreamTypeDASHVideo, StreamTypeDASHAudio} {
			checkStreamSpecified(t, codec.Name+" "+stream.Name, stream.Args(codec, 0, "", false, "out"))
		}
	}
}

// The specifier is the only change: values and order stay as before.
func TestCodecArgsUnchangedApartFromSpecifier(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  Args
		want Args
	}{
		{"N264H", VideoCodecN264H.ExtraArgs(), Args{
			"-tune", "hq", "-profile:v", "high", "-rc", "vbr", "-rc-lookahead", "60",
			"-surfaces", "64", "-spatial-aq", "1", "-aq-strength", "15", "-cq", "15",
			"-coder", "cabac", "-b_ref_mode", "middle",
		}},
		{"N264H HQ", VideoCodecN264H.ExtraArgsHQ("slow", 21, 4), Args{
			"-tune", "hq", "-profile:v", "high", "-rc", "vbr", "-rc-lookahead", "60",
			"-surfaces", "64", "-spatial-aq", "1", "-aq-strength", "15", "-cq", "10",
			"-coder", "cabac", "-b_ref_mode", "middle",
		}},
		{"I264", VideoCodecI264.ExtraArgs(), Args{"-global_quality:v", "20", "-preset", "faster"}},
		{"IVP9 HQ", VideoCodecIVP9.ExtraArgsHQ("slow", 21, 4), Args{"-global_quality:v", "15", "-preset", "slow"}},
		{"I264C", VideoCodecI264C.ExtraArgs(), Args{"-q:v", "20", "-preset", "faster"}},
		{"I264C HQ", VideoCodecI264C.ExtraArgsHQ("slow", 21, 4), Args{"-q:v", "15", "-preset", "slow"}},
		{"V264", VideoCodecV264.ExtraArgs(), Args{"-qp", "20"}},
		{"x264", VideoCodecLibX264.ExtraArgs(), Args{
			"-pix_fmt", "yuv420p", "-preset", "veryfast", "-crf", "25", "-sc_threshold", "0",
		}},
		{"x264 HQ", VideoCodecLibX264.ExtraArgsHQ("slow", 21, 4), Args{
			"-pix_fmt", "yuv420p", "-profile:v", "high", "-level:v", "4.2", "-preset", "slow",
			"-crf", "21", "-sc_threshold", "0", "-threads", "4", "-sws_flags", "lanczos",
		}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s args = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}
