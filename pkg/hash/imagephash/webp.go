package imagephash

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"

	"golang.org/x/image/riff"
	"golang.org/x/image/webp"
)

// loadAnimatedWebP decodes only the first frame, on its own, as image.Decode
// does for GIF. Go's WebP decoder only reads still images, so wrap the first
// ANMF's image chunks in a still WebP container before decoding them. The frame
// isn't drawn onto the animation's canvas: a phash doesn't need it, and a tiny
// file could otherwise claim a canvas of billions of pixels.
// https://developers.google.com/speed/webp/docs/riff_container#animation
func loadAnimatedWebP(r io.Reader) (image.Image, error) {
	_, chunks, err := riff.NewReader(r)
	if err != nil {
		return nil, err
	}

	var canvas image.Rectangle
	var seenAnimation bool
	for {
		id, size, data, err := chunks.Next()
		if err != nil {
			return nil, fmt.Errorf("reading animated WebP: %w", err)
		}
		switch string(id[:]) {
		case "VP8X":
			var header [10]byte
			if size != 10 {
				return nil, fmt.Errorf("invalid WebP extended header size")
			}
			if _, err := io.ReadFull(data, header[:]); err != nil {
				return nil, err
			}
			canvas = image.Rect(0, 0, webPUint24(header[4:7])+1, webPUint24(header[7:10])+1)
		case "ANIM":
			var header [6]byte
			if size != 6 {
				return nil, fmt.Errorf("invalid WebP animation header size")
			}
			if _, err := io.ReadFull(data, header[:]); err != nil {
				return nil, err
			}
			seenAnimation = true
		case "ANMF":
			if !seenAnimation || canvas.Empty() || size < 16 || size > 1<<32-7 {
				return nil, fmt.Errorf("invalid WebP frame header")
			}
			var frame [16]byte
			if _, err := io.ReadFull(data, frame[:]); err != nil {
				return nil, err
			}
			x, y := 2*webPUint24(frame[:3]), 2*webPUint24(frame[3:6])
			width, height := webPUint24(frame[6:9])+1, webPUint24(frame[9:12])+1
			rect := image.Rect(x, y, x+width, y+height)
			if !rect.In(canvas) {
				return nil, fmt.Errorf("WebP frame is outside canvas")
			}

			// ALPH needs a VP8X header with the frame's dimensions, rather
			// than the animation canvas dimensions. VP8L carries its own alpha.
			frameData := bufio.NewReader(data)
			firstChunk, err := frameData.Peek(4)
			if err != nil {
				return nil, err
			}
			var header [30]byte
			copy(header[:], "RIFF")
			binary.LittleEndian.PutUint32(header[4:8], size+6)
			copy(header[8:], "WEBPVP8X")
			binary.LittleEndian.PutUint32(header[16:20], 10)
			if string(firstChunk) == "ALPH" {
				header[20] = 0x10
			}
			copy(header[24:30], frame[6:12])
			img, err := webp.Decode(io.MultiReader(bytes.NewReader(header[:]), frameData))
			if err != nil {
				return nil, fmt.Errorf("decoding WebP frame: %w", err)
			}
			if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
				return nil, fmt.Errorf("WebP frame dimensions do not match header")
			}
			return img, nil
		}
	}
}

func webPUint24(b []byte) int {
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16
}
