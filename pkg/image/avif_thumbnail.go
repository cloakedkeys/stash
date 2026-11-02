package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"strings"

	"github.com/stashapp/stash/pkg/ffmpeg"
)

// AVIF stores alpha in a separate grayscale AV1 stream. Decode to PNG before
// choosing the thumbnail format, so neither decoder discards transparency.
func (e *ThumbnailEncoder) avifThumbnailPath(inputPath string, maxSize int) ([]byte, error) {
	var data []byte
	var err error
	if e.vips != nil {
		data, err = e.vips.imageThumbnailPath(inputPath, ".png[strip]", maxSize)
	} else {
		data, err = e.ffmpegAVIFThumbnailPNG(inputPath, maxSize)
	}
	if err != nil {
		return nil, err
	}

	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if opaque, ok := decoded.(interface{ Opaque() bool }); ok && !opaque.Opaque() {
		return e.transparentThumbnail(decoded, maxSize)
	}

	// Keep the existing JPEG encoder for images whose pixels are all opaque.
	if e.vips != nil {
		return e.vips.ImageThumbnailPath(inputPath, maxSize)
	}
	return e.ffmpegImageThumbnailPath(inputPath, maxSize)
}

func (e *ThumbnailEncoder) ffmpegAVIFThumbnailPNG(inputPath string, maxSize int) ([]byte, error) {
	probe, err := e.FFProbe.NewVideoFile(inputPath)
	if err != nil {
		return nil, err
	}
	colour := probe.VideoStream
	if colour == nil {
		return nil, fmt.Errorf("AVIF has no video stream: %s", inputPath)
	}

	filter := fmt.Sprintf("[0:%d]", colour.Index)
	for _, stream := range probe.JSON.Streams {
		if stream.Index != colour.Index && stream.CodecType == "video" &&
			stream.CodecName == "av1" && strings.HasPrefix(stream.PixFmt, "gray") &&
			stream.Width == colour.Width && stream.Height == colour.Height {
			filter += fmt.Sprintf("[0:%d]alphamerge,", stream.Index)
			break
		}
	}
	var scale ffmpeg.VideoFilter
	filter += string(scale.ScaleMaxSize(maxSize)) + "[thumb]"
	args := ffmpeg.Args{"-hide_banner", "-v", "error", "-i", inputPath,
		"-filter_complex", filter, "-map", "[thumb]", "-frames:v", "1",
		"-c:v", "png", "-f", "image2pipe", "-"}
	return e.FFMpeg.GenerateOutput(context.TODO(), args, nil)
}
