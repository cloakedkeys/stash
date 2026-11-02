package imagephash

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"

	"github.com/corona10/goimagehash"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/ffmpeg/transcoder"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
)

// Generate computes a perceptual hash for an image file.
func Generate(encoder *ffmpeg.FFMpeg, imageFile models.File) (*uint64, error) {
	img, err := loadImage(encoder, imageFile)
	if err != nil {
		return nil, fmt.Errorf("loading image: %w", err)
	}

	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return nil, fmt.Errorf("computing phash from image: %w", err)
	}

	hashValue := hash.GetHash()
	return &hashValue, nil
}

// loadImage loads an image from disk and decodes it.
// Where Go has no built-in decoder for a specific format, ffmpeg is used to convert to BMP first.
func loadImage(encoder *ffmpeg.FFMpeg, imageFile models.File) (image.Image, error) {
	// try to load with Go's built-in decoders first for better performance
	reader, err := imageFile.Open(&file.OsFS{})
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Animated WebP keeps its image chunks inside ANMF frames. Decode the
	// first frame directly; ffmpeg builds may not support this container.
	buffered := bufio.NewReader(reader)
	header, _ := buffered.Peek(21)
	if len(header) == 21 && string(header[:4]) == "RIFF" && string(header[8:16]) == "WEBPVP8X" && header[20]&2 != 0 {
		img, err := loadAnimatedWebP(buffered)
		if err != nil {
			return nil, fmt.Errorf("decoding image: %w", err)
		}
		return img, nil
	}

	img, _, err := image.Decode(buffered)
	if errors.Is(err, image.ErrFormat) {
		// try ffmpeg as a fallback for unsupported formats
		// ffmpeg cannot read files inside zips
		if imageFile.Base().ZipFileID != nil {
			return nil, fmt.Errorf("ffmpeg fallback unsupported for images in zip files")
		}
		return loadImageFFmpeg(encoder, imageFile.Base().Path)
	}

	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}

	return img, nil
}

// loadImageFFmpeg uses ffmpeg to convert an image to BMP and then decodes it.
func loadImageFFmpeg(encoder *ffmpeg.FFMpeg, path string) (image.Image, error) {
	options := transcoder.ScreenshotOptions{
		OutputPath: "-",
		OutputType: transcoder.ScreenshotOutputTypeBMP,
	}

	args := transcoder.ScreenshotTime(path, 0, options)
	data, err := encoder.GenerateOutput(context.Background(), args, nil)
	if err != nil {
		return nil, fmt.Errorf("converting image with ffmpeg: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding ffmpeg output: %w", err)
	}

	return img, nil
}
