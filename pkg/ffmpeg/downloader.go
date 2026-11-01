package ffmpeg

import (
	"runtime"
)

func GetFFmpegURL() []string {
	var urls []string
	switch runtime.GOOS {
	case "darwin":
		urls = []string{"https://evermeet.cx/ffmpeg/getrelease/zip", "https://evermeet.cx/ffmpeg/getrelease/ffprobe/zip"}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			urls = []string{"https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffmpeg-6.1-linux-64.zip", "https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffprobe-6.1-linux-64.zip"}
		case "arm":
			urls = []string{"https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffmpeg-6.1-linux-armhf-32.zip", "https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffprobe-6.1-linux-armhf-32.zip"}
		case "arm64":
			urls = []string{"https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffmpeg-6.1-linux-arm-64.zip", "https://github.com/ffbinaries/ffbinaries-prebuilt/releases/download/v6.1/ffprobe-6.1-linux-arm-64.zip"}
		}
	case "windows":
		// BtbN drops old release branches from latest; move this URL to a newer branch when 9.0 goes away.
		urls = []string{"https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n9.0-latest-win64-gpl-9.0.zip"}
	default:
		urls = []string{""}
	}
	return urls
}

func getFFMpegFilename() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

func getFFProbeFilename() string {
	if runtime.GOOS == "windows" {
		return "ffprobe.exe"
	}
	return "ffprobe"
}
