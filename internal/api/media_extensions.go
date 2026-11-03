package api

import (
	"fmt"
	"strings"
)

// The formats Stash can read, without the leading dot. A file can be more than
// one kind: an animated .gif or .webm can be a video or an image clip, and an
// .mp4 or .webm with only sound can be audio, so the lists overlap.
var (
	videoFormats = strings.Fields("m4v mp4 mov wmv avi mpg mpeg rmvb rm flv asf mkv webm f4v ts m2ts mts m2t vob ogv 3gp 3g2 mxf divx")
	audioFormats = strings.Fields("mp3 mpa m4a m4b aac flac wav aif aiff aifc ogg oga opus wma ape mka ac3 eac3 dts mp2 amr au caf wv")
	imageFormats = strings.Fields("png jpg jpeg jpe gif webp avif jxl bmp tif tiff apng")

	supportedExtensions = []struct {
		kind    string
		formats []string
	}{
		{"video", concat(videoFormats, []string{"gif", "webp", "apng"})},
		{"audio", concat(audioFormats, videoFormats)},
		{"image", concat(imageFormats, videoFormats)},
		{"story", strings.Fields("txt text md markdown")},
		{"gallery ZIP", strings.Fields("zip cbz")},
	}
)

func concat(lists ...[]string) []string {
	var ret []string
	for _, l := range lists {
		ret = append(ret, l...)
	}
	return ret
}

// validateMediaExtensions rejects an extension Stash can't read, before
// ConfigureGeneral changes anything (#1945). Settings already saved are left
// alone, so an older config still loads.
func validateMediaExtensions(input ConfigGeneralInput) error {
	lists := [][]string{input.VideoExtensions, input.AudioExtensions, input.ImageExtensions, input.StoryExtensions, input.GalleryExtensions}
	for i, s := range supportedExtensions {
		for _, extension := range lists[i] {
			if !containsFold(s.formats, extension) {
				return fmt.Errorf("%q isn't a format Stash can read as %s. Use extensions such as %s, without a dot", extension, s.kind, strings.Join(s.formats[:min(len(s.formats), 4)], ", "))
			}
		}
	}
	return nil
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
