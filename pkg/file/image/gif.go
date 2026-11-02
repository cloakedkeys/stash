package image

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"image/gif"
	"io"
	"math"

	"github.com/stashapp/stash/pkg/models"
)

func decorateZipGIF(fs models.FS, f models.File) (models.File, error) {
	r, err := fs.Open(f.Base().Path)
	if err != nil {
		return f, err
	}
	defer r.Close()

	reader := bufio.NewReader(r)
	config, err := gif.DecodeConfig(reader)
	if err != nil {
		return f, fmt.Errorf("reading GIF %q: %w", f.Base().Path, err)
	}
	if config.Width == 0 || config.Height == 0 {
		return f, fmt.Errorf("invalid GIF dimensions for %q", f.Base().Path)
	}

	// DecodeConfig consumes the header and global colour table. Read timing
	// blocks without DecodeAll: it retains every decompressed frame, which can
	// use gigabytes for a small compressed animation. This keeps scan memory
	// constant regardless of the frame count or dimensions, like ffprobe's GIF
	// demuxer. Pixel/LZW validation is left to the image decoder at display time.
	frames, ticks, err := gifTiming(reader, config.Width, config.Height)
	if err != nil {
		return f, fmt.Errorf("reading GIF %q: %w", f.Base().Path, err)
	}
	// Finish the ZIP entry so that its checksum is checked as well.
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return f, err
	}

	duration := float64(ticks) / 100
	// ffprobe's GIF demuxer uses an integer average delay in centiseconds.
	frameRate := math.Round(10000/float64(ticks/frames)) / 100
	return &models.VideoFile{
		BaseFile:   f.Base(),
		Format:     "gif",
		VideoCodec: "gif",
		Width:      config.Width,
		Height:     config.Height,
		Duration:   duration,
		FrameRate:  frameRate,
		BitRate:    int64(float64(f.Base().Size) * 8 / duration),
	}, nil
}

// gifTiming walks GIF blocks after DecodeConfig. Delays are hundredths of a
// second; a missing/zero delay uses ffprobe's default of 10 (one tenth second).
// Loop extensions do not multiply the duration, matching ffprobe's ignore_loop.
func gifTiming(r *bufio.Reader, width, height int) (frames, ticks int64, err error) {
	delay := int64(10)
	var data [9]byte
	for {
		var block byte
		block, err = r.ReadByte()
		if err != nil {
			return
		}
		switch block {
		case 0x21: // extension
			var label byte
			label, err = r.ReadByte()
			if err != nil {
				return
			}
			if label == 0xf9 { // graphic control extension
				if _, err = io.ReadFull(r, data[:6]); err != nil {
					return
				}
				if data[0] != 4 || data[5] != 0 {
					return 0, 0, fmt.Errorf("invalid GIF graphic control extension")
				}
				delay = int64(binary.LittleEndian.Uint16(data[2:4]))
				if delay == 0 {
					delay = 10
				}
			} else if err = skipGIFSubBlocks(r); err != nil {
				return
			}
		case 0x2c: // image descriptor
			if _, err = io.ReadFull(r, data[:]); err != nil {
				return
			}
			x := int(binary.LittleEndian.Uint16(data[0:2]))
			y := int(binary.LittleEndian.Uint16(data[2:4]))
			w := int(binary.LittleEndian.Uint16(data[4:6]))
			h := int(binary.LittleEndian.Uint16(data[6:8]))
			if w == 0 || h == 0 || x+w > width || y+h > height {
				return 0, 0, fmt.Errorf("invalid GIF frame dimensions")
			}
			if flags := data[8]; flags&0x80 != 0 {
				if _, err = r.Discard(3 * (1 << ((flags & 7) + 1))); err != nil {
					return
				}
			}
			var codeSize byte
			codeSize, err = r.ReadByte()
			if err != nil {
				return
			}
			if codeSize < 2 || codeSize > 8 {
				return 0, 0, fmt.Errorf("invalid GIF LZW code size")
			}
			if err = skipGIFSubBlocks(r); err != nil {
				return
			}
			frames++
			ticks += delay
			delay = 10
		case 0x3b: // trailer
			if frames == 0 {
				return 0, 0, fmt.Errorf("GIF has no frames")
			}
			return
		default:
			return 0, 0, fmt.Errorf("unknown GIF block %#x", block)
		}
	}
}

func skipGIFSubBlocks(r *bufio.Reader) error {
	for {
		n, err := r.ReadByte()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if _, err := r.Discard(int(n)); err != nil {
			return err
		}
	}
}
