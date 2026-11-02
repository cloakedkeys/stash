package image

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/ffmpeg/transcoder"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models"
	_ "golang.org/x/image/webp"
)

const (
	ffmpegImageQuality = 5
	clipPreviewCRF     = 25
)

// Limit full Go decodes to avoid excessive memory use in the server.
const maxThumbnailDecodePixels = 100_000_000

var vipsPath string
var once sync.Once

// ErrNotSupportedForThumbnail is returned if the image format is not supported for thumbnail generation
var ErrNotSupportedForThumbnail = errors.New("unsupported image format for thumbnail")

type ThumbnailEncoder struct {
	FFMpeg             *ffmpeg.FFMpeg
	FFProbe            *ffmpeg.FFProbe
	ClipPreviewOptions ClipPreviewOptions
	vips               *vipsEncoder
}

type ClipPreviewOptions struct {
	InputArgs  []string
	OutputArgs []string
	Preset     string
	HWAccel    bool
}

func GetVipsPath() string {
	once.Do(func() {
		vipsPath, _ = exec.LookPath("vips")
	})
	return vipsPath
}

func NewThumbnailEncoder(ffmpegEncoder *ffmpeg.FFMpeg, ffProbe *ffmpeg.FFProbe, clipPreviewOptions ClipPreviewOptions) ThumbnailEncoder {
	ret := ThumbnailEncoder{
		FFMpeg:             ffmpegEncoder,
		FFProbe:            ffProbe,
		ClipPreviewOptions: clipPreviewOptions,
	}

	vipsPath := GetVipsPath()
	if vipsPath != "" {
		vipsEncoder := vipsEncoder(vipsPath)
		ret.vips = &vipsEncoder
	}

	return ret
}

// GetThumbnail returns the thumbnail image of the provided image resized to
// the provided max size. It resizes based on the largest X/Y direction.
// Opaque images use JPEG; transparent images use WebP, falling back to PNG
// if WebP encoding is unavailable.
// It returns nil and an error if an error occurs reading, decoding or encoding
// the image, or if the image is not suitable for thumbnails.
func (e *ThumbnailEncoder) GetThumbnail(f models.File, maxSize int) ([]byte, error) {
	// Video-backed image clips can be large, so avoid buffering the whole file.
	if _, ok := f.(*models.VideoFile); ok {
		return e.ffmpegImageThumbnailPath(f.Base().Path, maxSize)
	}

	reader, err := f.Open(&file.OsFS{})
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(reader); err != nil {
		return nil, err
	}

	data := buf.Bytes()

	format := ""
	if imageFile, ok := f.(*models.ImageFile); ok {
		format = imageFile.Format
		animated := imageFile.Format == formatGif

		// #2266 - if image is webp, then determine if it is animated
		if format == formatWebP {
			animated = isWebPAnimated(data)
		}

		// #2266 - don't generate a thumbnail for animated images
		if animated {
			return nil, fmt.Errorf("%w: %s", ErrNotSupportedForThumbnail, format)
		}

		// AVIF cannot be read from stdin, must use file path
		// AVIF in zip files is not supported
		// Note: No Windows check needed here since we use file path, not stdin
		if format == "avif" {
			if f.Base().ZipFileID != nil {
				return nil, fmt.Errorf("%w: AVIF in zip file", ErrNotSupportedForThumbnail)
			}
		}
	}

	if format == "avif" {
		if e.vips != nil {
			return e.vips.ImageThumbnailPath(f.Base().Path, maxSize)
		}
		return e.ffmpegImageThumbnailPath(f.Base().Path, maxSize)
	}

	// Inspect actual alpha values, so an opaque image with an alpha channel
	// still uses the existing JPEG encoder.
	if format != "mjpeg" && format != "jpeg" && shouldDecodeThumbnail(data) {
		decoded, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
		if err == nil {
			if opaque, ok := decoded.(interface{ Opaque() bool }); ok && !opaque.Opaque() {
				return e.transparentThumbnail(decoded, maxSize)
			}
		}
	}

	// vips has issues loading files from stdin on Windows
	if e.vips != nil {
		if runtime.GOOS == "windows" && f.Base().ZipFileID == nil {
			return e.vips.ImageThumbnailPath(f.Base().Path, maxSize)
		}
		if runtime.GOOS != "windows" {
			return e.vips.ImageThumbnail(buf, maxSize)
		}
	}
	return e.ffmpegImageThumbnail(buf, maxSize)
}

func shouldDecodeThumbnail(data []byte) bool {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return false
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxThumbnailDecodePixels/config.Height {
		return false
	}

	// PNG reports RGB(A) models for truecolour files without alpha, while
	// BMP and TIFF can also use those models for files with alpha.
	if format == "png" && (config.ColorModel == color.RGBAModel || config.ColorModel == color.RGBA64Model) {
		return false
	}
	// Lossy WebP without alpha reports YCbCr; with alpha it reports NYCbCrA.
	// Lossless WebP reports NRGBA even when all pixels are opaque.
	switch config.ColorModel {
	case color.GrayModel, color.Gray16Model, color.YCbCrModel, color.CMYKModel:
		return false
	}
	return true
}

func (e *ThumbnailEncoder) transparentThumbnail(decoded image.Image, maxSize int) ([]byte, error) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, imaging.Fit(decoded, maxSize, maxSize, imaging.Lanczos)); err != nil {
		return nil, err
	}

	// Use the configured ffmpeg, including the downloaded library binary.
	// A system build may lack libwebp; the PNG remains a usable thumbnail.
	args := ffmpeg.Args{"-hide_banner", "-v", "error", "-i", "-", "-frames:v", "1",
		"-c:v", "libwebp", "-lossless", "1", "-pix_fmt", "bgra", "-f", "webp", "-"}
	data, err := e.FFMpeg.GenerateOutput(context.TODO(), args, bytes.NewReader(pngData.Bytes()))
	if err != nil {
		return pngData.Bytes(), nil
	}
	return data, nil
}

// GetPreview returns the preview clip of the provided image clip resized to
// the provided max size. It resizes based on the largest X/Y direction.
// It is hardcoded to 30 seconds maximum right now
func (e *ThumbnailEncoder) GetPreview(inPath string, outPath string, maxSize int) error {
	fileData, err := e.FFProbe.NewVideoFile(inPath)
	if err != nil {
		return err
	}
	if fileData.Width <= maxSize {
		maxSize = fileData.Width
	}
	clipDuration := fileData.VideoStreamDuration
	if clipDuration > 30.0 {
		clipDuration = 30.0
	}
	return e.getClipPreview(inPath, outPath, maxSize, clipDuration, fileData.FrameRate, fileData)
}

func (e *ThumbnailEncoder) ffmpegImageThumbnail(image *bytes.Buffer, maxSize int) ([]byte, error) {
	options := transcoder.ImageThumbnailOptions{
		OutputFormat:  ffmpeg.ImageFormatJpeg,
		OutputPath:    "-",
		MaxDimensions: maxSize,
		Quality:       ffmpegImageQuality,
	}

	args := transcoder.ImageThumbnail("-", options)

	return e.FFMpeg.GenerateOutput(context.TODO(), args, image)
}

// ffmpegImageThumbnailPath generates a thumbnail from a file path (used for AVIF which can't be piped)
func (e *ThumbnailEncoder) ffmpegImageThumbnailPath(inputPath string, maxSize int) ([]byte, error) {
	options := transcoder.ImageThumbnailOptions{
		OutputFormat:  ffmpeg.ImageFormatJpeg,
		OutputPath:    "-",
		MaxDimensions: maxSize,
		Quality:       ffmpegImageQuality,
	}

	args := transcoder.ImageThumbnail(inputPath, options)

	return e.FFMpeg.GenerateOutput(context.TODO(), args, nil)
}

func (e *ThumbnailEncoder) getClipPreview(inPath string, outPath string, maxSize int, clipDuration float64, frameRate float64, fileData *ffmpeg.VideoFile) error {
	codec := ffmpeg.VideoCodecVP9
	if e.ClipPreviewOptions.HWAccel {
		codec = e.FFMpeg.HWCodecWEBMCompatible(codec)
	}

	var thumbFilter ffmpeg.VideoFilter
	// TODO: switching this to a HW-compatible scale filter would let us enable
	// full hardware encoding here. ScaleMaxSize uses force_original_aspect_ratio
	// which the HW scaler templates don't handle, so we fall back to non-fullhw.
	thumbFilter = thumbFilter.ScaleMaxSize(maxSize)

	o := e.ClipPreviewOptions

	thumbArgs := codec.ExtraArgsHQ(o.Preset, clipPreviewCRF)
	thumbArgs = thumbArgs.VideoFilter(thumbFilter)

	thumbArgs = append(thumbArgs,
		"-f", "webm",
	)

	if frameRate <= 0.01 {
		thumbArgs = thumbArgs.VSync(ffmpeg.VSyncMethodVFR, e.FFMpeg.Version())
	}

	extraInputArgs := e.FFMpeg.HWDeviceInit(o.InputArgs, codec, false)

	thumbOptions := transcoder.TranscodeOptions{
		OutputPath: outPath,
		StartTime:  0,
		Duration:   clipDuration,

		XError:   true,
		SlowSeek: false,

		VideoCodec: codec,
		VideoArgs:  thumbArgs,

		ExtraInputArgs:  extraInputArgs,
		ExtraOutputArgs: o.OutputArgs,
	}

	if err := fsutil.EnsureDirAll(filepath.Dir(outPath)); err != nil {
		return err
	}
	args := transcoder.Transcode(inPath, thumbOptions)
	return e.FFMpeg.Generate(context.TODO(), args)
}
