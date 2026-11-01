package videophash

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/bits"

	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/ffmpeg/transcoder"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

const screenshotSize = 160

// spriteColumns returns the width and height in frames of the phash sprite grid.
// Short videos use fewer frames: sampled close together they are near-identical,
// and the repetitive sprite that results hashes alike across unrelated videos.
// See https://github.com/stashapp/stash/issues/3722.
func spriteColumns(duration float64) int {
	switch {
	case duration <= 45:
		return 2
	case duration <= 90:
		return 3
	case duration <= 150:
		return 4
	default:
		return 5
	}
}

func Generate(encoder *ffmpeg.FFMpeg, videoFile *models.VideoFile) (*uint64, error) {
	if videoFile.Duration <= 0 {
		return nil, fmt.Errorf("cannot generate phash with nonpositive video duration: %v", videoFile.Duration)
	}

	sprite, err := generateSprite(encoder, videoFile, spriteColumns(videoFile.Duration))
	if err != nil {
		return nil, err
	}

	hash, err := goimagehash.PerceptionHash(sprite)
	if err != nil {
		return nil, fmt.Errorf("computing phash from sprite: %w", err)
	}
	hashValue := hash.GetHash()
	if !validHash(hashValue) {
		return nil, fmt.Errorf("invalid phash: %016x matches a known bad phash", hashValue)
	}
	return &hashValue, nil
}

func validHash(hash uint64) bool {
	// Solid black and white sprites produce 0 and 8000000000000000.
	// The other values are reported in https://github.com/stashapp/stash/issues/2149.
	// Allow up to three changed bits to catch variations of these bad hashes.
	for _, bad := range [...]uint64{
		0x0000000000000000,
		0x8000000000000000,
		0xa000000000800080,
		0x8080808080808080,
		0x870707030787fefc,
		0x82070707078ffff8,
		0x8055557555575575,
		0x805555555d755d55,
		0x87070707037ef8fc,
		0x8707070303fefcdc,
		0xcdcdcdc9c1233332,
		0xf8fcfcfcf8e00100,
		0xf8fcfcfcfc700000,
	} {
		if bits.OnesCount64(hash^bad) <= 3 {
			return false
		}
	}
	return true
}

func generateSpriteScreenshot(encoder *ffmpeg.FFMpeg, input string, t float64, slowSeek bool) (image.Image, error) {
	options := transcoder.ScreenshotOptions{
		Width:      screenshotSize,
		OutputPath: "-",
		OutputType: transcoder.ScreenshotOutputTypeBMP,
		SlowSeek:   slowSeek,
	}

	args := transcoder.ScreenshotTime(input, t, options)
	data, err := encoder.GenerateOutput(context.Background(), args, nil)
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(data)

	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}

	return img, nil
}

func combineImages(images []image.Image, columns int) image.Image {
	width := images[0].Bounds().Size().X
	height := images[0].Bounds().Size().Y
	canvasWidth := width * columns
	canvasHeight := height * columns
	montage := imaging.New(canvasWidth, canvasHeight, color.NRGBA{})
	for index := 0; index < len(images); index++ {
		x := width * (index % columns)
		y := height * int(math.Floor(float64(index)/float64(columns)))
		img := images[index]
		montage = imaging.Paste(montage, img, image.Pt(x, y))
	}

	return montage
}

func generateSprite(encoder *ffmpeg.FFMpeg, videoFile *models.VideoFile, columns int) (image.Image, error) {
	logger.Infof("[generator] generating phash sprite for %s", videoFile.Path)

	// Generate sprite image offset by 5% on each end to avoid intro/outros
	chunkCount := columns * columns
	offset := 0.05 * videoFile.Duration
	stepSize := (0.9 * videoFile.Duration) / float64(chunkCount)
	var images []image.Image
	slowSeek := false

	for i := 0; i < chunkCount; i++ {
		time := offset + (float64(i) * stepSize)

		img, err := generateSpriteScreenshot(encoder, videoFile.Path, time, slowSeek)
		if err != nil && !slowSeek {
			logger.Warnf("[generator] fast phash screenshot seek failed for %s at %.3fs, retrying with accurate seek for remaining phash screenshots: %v", videoFile.Path, time, err)

			slowSeek = true
			img, err = generateSpriteScreenshot(encoder, videoFile.Path, time, slowSeek)
		}
		if err != nil {
			return nil, fmt.Errorf("generating sprite screenshot: %w", err)
		}

		images = append(images, img)
	}

	// Combine all of the thumbnails into a sprite image
	if len(images) == 0 {
		return nil, fmt.Errorf("images slice is empty, failed to generate phash sprite for %s", videoFile.Path)
	}

	return combineImages(images, columns), nil
}
