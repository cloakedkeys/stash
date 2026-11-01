package utils

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/logger"
)

// Timeout to get the image. Includes transfer time. May want to make this
// configurable at some point.
const imageGetTimeout = time.Second * 60

const base64RE = `^data:.+\/(.+);base64,(.*)$`

var base64Regex = regexp.MustCompile(base64RE)

// ProcessImageInput transforms an image string either from a base64 encoded
// string, or from a URL, and returns the image as a byte slice
func ProcessImageInput(ctx context.Context, imageInput string) ([]byte, error) {
	if imageInput == "" {
		return []byte{}, nil
	}

	if base64Regex.MatchString(imageInput) {
		d, err := ProcessBase64Image(imageInput)
		return d, err
	}

	// assume input is a URL. Read it.
	d, err := ReadImageFromURL(ctx, imageInput)
	if err != nil {
		return nil, err
	}

	if err := validateImageData(d); err != nil {
		return nil, err
	}

	return d, nil
}

var errNonSVGImageText = errors.New("image text must have an <svg> root element")

// validateImageData rejects HTML and non-SVG text. Binary formats are left alone.
func validateImageData(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	contentType := http.DetectContentType(data)
	if strings.HasPrefix(contentType, "text/html") {
		return fmt.Errorf("unsupported image content type %q", contentType)
	}
	if contentType == "text/plain; charset=utf-8" || contentType == "text/xml; charset=utf-8" {
		// Attribute text may use the declared encoding. Replace invalid UTF-8
		// only in the parser's input, since attribute values do not affect the root name.
		text := bytes.ToValidUTF8(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), []byte("\uFFFD"))
		decoder := xml.NewDecoder(bytes.NewReader(text))
		// Only the root name matters; leave declared encodings and image bytes alone.
		decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) {
			return input, nil
		}
		for {
			token, err := decoder.RawToken()
			if err != nil {
				return fmt.Errorf("%w (%s): %v", errNonSVGImageText, contentType, err)
			}
			switch token := token.(type) {
			case xml.StartElement:
				if token.Name.Local == "svg" {
					return nil
				}
				return fmt.Errorf("%w (%s)", errNonSVGImageText, contentType)
			case xml.CharData:
				if len(bytes.TrimSpace(token)) != 0 {
					return fmt.Errorf("%w (%s)", errNonSVGImageText, contentType)
				}
			}
		}
	}
	if strings.HasPrefix(contentType, "text/") {
		return fmt.Errorf("%w (%s)", errNonSVGImageText, contentType)
	}

	return nil
}

// ReadImageFromURL returns image data from a URL
func ReadImageFromURL(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{
		Transport: &http.Transport{ // ignore insecure certificates
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy:           http.ProxyFromEnvironment,
		},

		Timeout: imageGetTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// assume is a URL for now

	// set the host of the URL as the referer
	if req.URL.Scheme != "" {
		req.Header.Set("Referer", req.URL.Scheme+"://"+req.Host+"/")
	}
	req.Header.Set("User-Agent", getUserAgent())

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http error %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// ProcessBase64Image transforms a base64 encoded string from a form post and
// returns the image itself as a byte slice.
func ProcessBase64Image(imageString string) ([]byte, error) {
	if imageString == "" {
		return nil, fmt.Errorf("empty image string")
	}

	matches := base64Regex.FindStringSubmatch(imageString)
	var encodedString string
	if len(matches) > 2 {
		encodedString = matches[2]
	} else {
		encodedString = imageString
	}
	imageData, err := GetDataFromBase64String(encodedString)
	if err != nil {
		return nil, err
	}

	if err := validateImageData(imageData); err != nil {
		return nil, err
	}

	return imageData, nil
}

// ProcessBase64ImageForImport skips text images accepted by older versions so
// restoring an old backup still keeps the object's other metadata.
// A skipped image returns nil data and no error.
// Existing base64 and HTML errors continue to be reported to the importer.
func ProcessBase64ImageForImport(imageString string) ([]byte, error) {
	data, err := ProcessBase64Image(imageString)
	if errors.Is(err, errNonSVGImageText) {
		logger.Warnf("Skipping image during metadata import: %v", err)
		return nil, nil
	}
	return data, err
}

// GetDataFromBase64String returns the given base64 encoded string as a byte slice
func GetDataFromBase64String(encodedString string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(encodedString)
}

// GetBase64StringFromData returns the given byte slice as a base64 encoded string
func GetBase64StringFromData(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func ServeImage(w http.ResponseWriter, r *http.Request, image []byte) {
	contentType := http.DetectContentType(image)

	// SVG images are detected as XML or plain text; serve them as SVG so they
	// render. The sandboxing CSP below prevents any embedded script running.
	if contentType == "text/xml; charset=utf-8" || contentType == "text/plain; charset=utf-8" {
		contentType = "image/svg+xml"
	} else if strings.HasPrefix(contentType, "text/") {
		// any other text type (e.g. HTML) is not a valid image - never render it
		contentType = "application/octet-stream"
		w.Header().Set("Content-Disposition", "attachment")
	}

	// sandbox every image response so a stored SVG cannot execute script or
	// exfiltrate data; harmless for raster images.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.Header().Set("Content-Type", contentType)
	ServeStaticContent(w, r, image)
}
