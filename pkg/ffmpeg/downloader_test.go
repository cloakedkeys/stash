package ffmpeg

import (
	"runtime"
	"testing"
)

func TestGetFFmpegURLWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows download URL")
	}

	const want = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n9.0-latest-win64-gpl-9.0.zip"
	got := GetFFmpegURL()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("GetFFmpegURL() = %v, want [%s]", got, want)
	}
}
