package api

import (
	"fmt"
	"strings"
)

// The formats Stash can read, without the leading dot. A file can be more than
// one kind: an animated .gif or .webm can be a video or an image clip, and an
// .mp4 or .webm with only sound can be audio, so the lists overlap.
var (
	videoFormats = strings.Fields("m4v mp4 mov qt wmv avi mpg mpeg mpe m1v m2v mp2v mpv rmvb rm flv asf mkv mk3d webm f4v ts m2ts mts m2t trp tp vob ogv ogg ogm 3gp 3g2 mxf divx xvid dv nut y4m wtv dvr-ms tod mod")
	audioFormats = strings.Fields("mp3 mpa m4a m4b m4r aac flac alac wav wave aif aiff aifc ogg oga spx opus wma ape mka ac3 eac3 dts mp2 mp1 amr au snd caf wv mpc tta dsf dff")
	imageFormats = strings.Fields("png jpg jpeg jpe jfif pjpeg pjp gif webp avif jxl bmp dib tif tiff apng")

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
// ConfigureGeneral changes anything (#1945). An extension already in the
// saved list stays allowed, so an older config can still be edited without
// losing a format someone relies on.
func validateMediaExtensions(input, saved ConfigGeneralInput) error {
	lists := [][]string{input.VideoExtensions, input.AudioExtensions, input.ImageExtensions, input.StoryExtensions, input.GalleryExtensions}
	savedLists := [][]string{saved.VideoExtensions, saved.AudioExtensions, saved.ImageExtensions, saved.StoryExtensions, saved.GalleryExtensions}
	for i, s := range supportedExtensions {
		for _, extension := range lists[i] {
			if !containsFold(s.formats, extension) && !containsFold(savedLists[i], extension) {
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
