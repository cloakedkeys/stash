package scraper

import (
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/pkg/models"
)

type queryURLReplacements map[string]mappedRegexConfigs

type queryURLParameters map[string]string

func queryURLParametersFromScene(scene *models.Scene) queryURLParameters {
	ret := make(queryURLParameters)
	ret["checksum"] = scene.Checksum
	ret["oshash"] = scene.OSHash
	ret["filename"] = filepath.Base(scene.Path)
	ret["path"] = scene.Path

	// pull phash from primary file
	phashFingerprints := scene.Files.Primary().Base().Fingerprints.Filter(models.FingerprintTypePhash)
	if len(phashFingerprints) > 0 {
		ret["phash"] = phashFingerprints[0].Value()
	}

	if scene.Title != "" {
		ret["title"] = scene.Title
	}
	if len(scene.URLs.List()) > 0 {
		ret["url"] = scene.URLs.List()[0]
	}
	return ret
}

func queryURLParametersFromScrapedScene(scene models.ScrapedSceneInput) queryURLParameters {
	ret := make(queryURLParameters)

	setField := func(field string, value *string) {
		if value != nil {
			ret[field] = *value
		}
	}

	setField("title", scene.Title)
	setField("code", scene.Code)
	if len(scene.URLs) > 0 {
		setField("url", &scene.URLs[0])
	} else {
		setField("url", scene.URL)
	}
	setField("date", scene.Date)
	setField("production_date", scene.ProductionDate)
	setField("details", scene.Details)
	setField("director", scene.Director)
	setField("remote_site_id", scene.RemoteSiteID)
	return ret
}

func queryURLParameterFromURL(url string) queryURLParameters {
	ret := make(queryURLParameters)
	ret["url"] = url
	return ret
}

func queryURLParametersFromGallery(gallery *models.Gallery) queryURLParameters {
	ret := make(queryURLParameters)
	ret["checksum"] = gallery.PrimaryChecksum()

	if gallery.Path != "" {
		ret["filename"] = filepath.Base(gallery.Path)
	}
	if gallery.Title != "" {
		ret["title"] = gallery.Title
	}

	if len(gallery.URLs.List()) > 0 {
		ret["url"] = gallery.URLs.List()[0]
	}

	return ret
}

func queryURLParametersFromImage(image *models.Image) queryURLParameters {
	ret := make(queryURLParameters)
	ret["checksum"] = image.Checksum

	if image.Path != "" {
		ret["filename"] = filepath.Base(image.Path)
	}
	if image.Title != "" {
		ret["title"] = image.Title
	}

	if len(image.URLs.List()) > 0 {
		ret["url"] = image.URLs.List()[0]
	}

	return ret
}

func (p queryURLParameters) applyReplacements(r queryURLReplacements) {
	for k, v := range p {
		rpl, found := r[k]
		if found {
			p[k] = rpl.apply(v)
		}
	}
}

func (p queryURLParameters) constructURL(url string) string {
	ret := url
	for k, v := range p {
		ret = strings.ReplaceAll(ret, "{"+k+"}", v)
	}

	return escapeInvalidURLChars(ret)
}

// escapeInvalidURLChars percent-encodes the characters that can never appear in a URL as-is,
// such as spaces and non-ASCII characters from a substituted filename or path.
// Characters with a meaning in URLs (& = + # / and valid % escapes) are left alone, so that
// templates and queryURLReplace rules that already encode their values keep working.
// The scheme and host are not changed.
func escapeInvalidURLChars(s string) string {
	start := 0
	if i := strings.Index(s, "://"); i >= 0 {
		start = i + 3
		if j := strings.IndexAny(s[start:], "/?#"); j >= 0 {
			start += j
		} else {
			start = len(s)
		}
	}

	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	b.WriteString(s[:start])
	for i := start; i < len(s); i++ {
		c := s[i]
		validEscape := c == '%' && i+2 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2])
		if c <= ' ' || c >= 0x7f || c == '"' || c == '<' || c == '>' || (c == '%' && !validEscape) {
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xf])
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isHexDigit(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

// replaceURL does a partial URL Replace ( only url parameter is used)
func replaceURL(url string, scraperConfig ByURLDefinition) string {
	u := url
	queryURL := queryURLParameterFromURL(u)
	if scraperConfig.QueryURLReplacements != nil {
		queryURL.applyReplacements(scraperConfig.QueryURLReplacements)
		u = queryURL.constructURL(scraperConfig.QueryURL)
	}
	return u
}
