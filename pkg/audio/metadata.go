package audio

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// Metadata is the metadata embedded in an audio file.
type Metadata struct {
	Title   string
	Details string
	Date    *models.Date
	Artists []string
	Genres  []string
	Album   string
	Track   *int
	Cover   []byte
}

// MetadataReader reads the metadata embedded in an audio file.
type MetadataReader interface {
	ReadMetadata(ctx context.Context, path string) (*Metadata, error)
}

// FileMetadataReader reads embedded metadata using ffprobe and ffmpeg.
type FileMetadataReader struct {
	FFProbe *ffmpeg.FFProbe
	FFMpeg  *ffmpeg.FFMpeg
}

func (r *FileMetadataReader) ReadMetadata(ctx context.Context, path string) (*Metadata, error) {
	probed, err := r.FFProbe.NewAudioFile(path)
	if err != nil {
		return nil, fmt.Errorf("running ffprobe on %q: %w", path, err)
	}

	ret := metadataFromTags(probed.Tags)

	if probed.HasCoverArt {
		cover, err := r.FFMpeg.ExtractAudioCover(ctx, path)
		if err != nil {
			// the rest of the metadata is still useful
			logger.Warnf("error extracting cover image from %q: %v", path, err)
		} else {
			ret.Cover = cover
		}
	}

	return ret, nil
}

func firstTag(tags map[string]string, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(tags[n]); v != "" {
			return v
		}
	}
	return ""
}

func metadataFromTags(tags map[string]string) *Metadata {
	return &Metadata{
		Title:   firstTag(tags, "title"),
		Details: firstTag(tags, "comment", "description"),
		Date:    parseTagDate(firstTag(tags, "date", "originaldate", "year")),
		Artists: splitTagValues(firstTag(tags, "artist", "album_artist", "albumartist")),
		Genres:  splitTagValues(firstTag(tags, "genre")),
		Album:   firstTag(tags, "album"),
		Track:   parseTagTrack(firstTag(tags, "track", "tracknumber")),
	}
}

// splitTagValues splits a tag that holds multiple values. Commas are left alone
// since they are common in names.
func splitTagValues(v string) []string {
	var ret []string
	for _, s := range strings.FieldsFunc(v, func(r rune) bool { return r == ';' || r == '/' }) {
		if s = strings.TrimSpace(s); s != "" {
			ret = append(ret, s)
		}
	}
	return ret
}

var tagDateRE = regexp.MustCompile(`^(\d{4})(?:[-/.]?(\d{2})(?:[-/.]?(\d{2}))?)?`)

// parseTagDate parses the date formats used in tags, returning nil if the value
// isn't a date. Only the date part of a timestamp is used.
func parseTagDate(v string) *models.Date {
	m := tagDateRE.FindStringSubmatch(v)
	if m == nil {
		return nil
	}

	s := m[1]
	if m[2] != "" {
		s += "-" + m[2]
		if m[3] != "" {
			s += "-" + m[3]
		}
	}

	d, err := models.ParseDate(s)
	if err != nil {
		return nil
	}
	return &d
}

// parseTagTrack parses a track number, which may be in the form of 3/12.
func parseTagTrack(v string) *int {
	v, _, _ = strings.Cut(v, "/")
	track, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || track < 0 {
		return nil
	}
	return &track
}
