package utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// a minimal valid 1x1 PNG
var pngImage = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestServeImage(t *testing.T) {
	const wantCSP = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox"

	tests := []struct {
		name            string
		image           []byte
		wantContentType string
		wantAttachment  bool
	}{
		{
			name:            "png is served as-is",
			image:           pngImage,
			wantContentType: "image/png",
		},
		{
			name:            "svg is served as image/svg+xml so it renders",
			image:           []byte(`<svg xmlns="http://www.w3.org/2000/svg"><circle r="1"/></svg>`),
			wantContentType: "image/svg+xml",
		},
		{
			name:            "svg with xml prolog is served as image/svg+xml",
			image:           []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`),
			wantContentType: "image/svg+xml",
		},
		{
			name:            "html is never rendered - forced to an attachment",
			image:           []byte(`<!DOCTYPE html><html><script>alert(1)</script></html>`),
			wantContentType: "application/octet-stream",
			wantAttachment:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/image", nil)

			ServeImage(w, r, tt.image)

			if got := w.Header().Get("Content-Type"); got != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantContentType)
			}
			if got := w.Header().Get("Content-Security-Policy"); got != wantCSP {
				t.Errorf("Content-Security-Policy = %q, want %q", got, wantCSP)
			}
			if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
			}
			gotAttachment := strings.Contains(w.Header().Get("Content-Disposition"), "attachment")
			if gotAttachment != tt.wantAttachment {
				t.Errorf("Content-Disposition attachment = %v, want %v (got %q)", gotAttachment, tt.wantAttachment, w.Header().Get("Content-Disposition"))
			}
		})
	}
}

func TestValidateImageData(t *testing.T) {
	utf16LE := []byte{0xff, 0xfe}
	utf16BE := []byte{0xfe, 0xff}
	for _, b := range []byte(`<?xml version="1.0" encoding="UTF-16"?><svg/>`) {
		utf16LE = append(utf16LE, b, 0)
		utf16BE = append(utf16BE, 0, b)
	}
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "empty is allowed", data: nil, wantErr: false},
		{name: "plain text is rejected", data: []byte("not an image"), wantErr: true},
		{name: "text mentioning svg is rejected", data: []byte("example: <svg></svg>"), wantErr: true},
		{name: "non SVG XML is rejected", data: []byte(`<?xml version="1.0"?><document><svg/></document>`), wantErr: true},
		{name: "SVG in comment is rejected", data: []byte(`<!-- <svg/> --><document/>`), wantErr: true},
		{name: "SVG named root is rejected", data: []byte(`<svgExample/>`), wantErr: true},
		{name: "comment only is rejected", data: []byte(`<!-- <svg/> -->`), wantErr: true},
		{name: "malformed XML is rejected", data: []byte(`<svg`), wantErr: true},
		{name: "png is allowed", data: pngImage, wantErr: false},
		{name: "unknown binary is allowed", data: []byte{0, 1, 2, 3, 255}, wantErr: false},
		{name: "svg is allowed", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), wantErr: false},
		{name: "xml declaration is allowed", data: []byte(`<?xml version="1.0"?><svg/>`), wantErr: false},
		{name: "leading comment is rejected as HTML", data: []byte(`<!-- image --><svg/>`), wantErr: true},
		{name: "comment after XML declaration is allowed", data: []byte(`<?xml version="1.0"?><!-- image --><svg/>`), wantErr: false},
		{name: "declared encoding is allowed", data: []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><svg/>`), wantErr: false},
		{name: "non ASCII root attribute is allowed", data: []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><svg title=\"caf\xe9\"/>"), wantErr: false},
		{name: "doctype is allowed", data: []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd"><svg/>`), wantErr: false},
		{name: "whitespace is allowed", data: []byte(" \t\r\n<svg/>"), wantErr: false},
		{name: "UTF8 BOM is allowed", data: []byte("\xef\xbb\xbf<?xml version=\"1.0\"?><svg/>"), wantErr: false},
		{name: "namespace prefix is allowed", data: []byte(`<s:svg xmlns:s="http://www.w3.org/2000/svg"/>`), wantErr: false},
		{name: "UTF16 LE SVG is rejected", data: utf16LE, wantErr: true},
		{name: "UTF16 BE SVG is rejected", data: utf16BE, wantErr: true},
		{name: "UTF16 without BOM remains binary", data: utf16LE[2:], wantErr: false},
		{name: "UTF16 text is rejected", data: []byte{0xff, 0xfe, 'n', 0, 'o', 0}, wantErr: true},
		// XML parsing stops at the root without validating the body.
		{name: "large body is not parsed", data: []byte("<svg>" + strings.Repeat("&invalid;", 1<<20)), wantErr: false},
		{name: "html is rejected", data: []byte(`<!DOCTYPE html><html></html>`), wantErr: true},
		{name: "html without doctype is rejected", data: []byte(`<html><body>x</body></html>`), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImageData(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateImageData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestImageInputRejectsText(t *testing.T) {
	data := []byte("example: <svg/>")
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	if _, err := ProcessBase64Image(encoded); err == nil {
		t.Fatal("ProcessBase64Image accepted text")
	}
	if _, err := ProcessImageInput(context.Background(), encoded); err == nil {
		t.Fatal("ProcessImageInput accepted base64 text")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()
	if _, err := ProcessImageInput(context.Background(), server.URL); err == nil {
		t.Fatal("ProcessImageInput accepted text from URL")
	}
}

func TestProcessBase64ImageForImport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		want    []byte
		wantErr bool
	}{
		{name: "legacy text is skipped", data: []byte("old broken image")},
		{name: "legacy XML is skipped", data: []byte("<document/>")},
		{name: "UTF16 LE is skipped", data: []byte{0xff, 0xfe, '<', 0, 's', 0, 'v', 0, 'g', 0, '/', 0, '>', 0}},
		{name: "UTF16 BE is skipped", data: []byte{0xfe, 0xff, 0, '<', 0, 's', 0, 'v', 0, 'g', 0, '/', 0, '>'}},
		{name: "PNG is kept", data: pngImage, want: pngImage},
		{name: "SVG is kept", data: []byte("<svg/>"), want: []byte("<svg/>")},
		{name: "HTML still fails", data: []byte("<html></html>"), wantErr: true},
		{name: "leading SVG comment still fails", data: []byte("<!-- image --><svg/>"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ProcessBase64ImageForImport(base64.StdEncoding.EncodeToString(tc.data))
			if (err != nil) != tc.wantErr || !bytes.Equal(got, tc.want) {
				t.Fatalf("got %q, %v; want %q, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
	if _, err := ProcessBase64ImageForImport("invalid base64!"); err == nil {
		t.Fatal("invalid base64 must still fail")
	}
}
