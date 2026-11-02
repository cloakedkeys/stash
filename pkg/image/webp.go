package image

const (
	formatWebP = "webp"
	formatGif  = "gif"
)

// https://developers.google.com/speed/webp/docs/riff_container
func isWebPAnimated(buf []byte) bool {
	const (
		flagsOffset = 20
		animBit     = 1 << 1
	)

	// Only VP8X has feature flags; VP8 and VP8L contain image data here.
	return len(buf) > flagsOffset &&
		string(buf[:4]) == "RIFF" &&
		string(buf[8:16]) == "WEBPVP8X" &&
		buf[flagsOffset]&animBit != 0
}
