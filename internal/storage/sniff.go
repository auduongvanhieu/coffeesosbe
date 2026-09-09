package storage

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
)

var svgRoot = regexp.MustCompile(`(?is)<svg[\s>]`)

// sniff detects the image type from the first bytes and rewinds the file.
// http.DetectContentType does not know SVG (it reports text/xml or
// text/plain), so a small check for an <svg root is added.
func sniff(f multipart.File) (string, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	head = head[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	ct := http.DetectContentType(head)
	switch ct {
	case "text/xml; charset=utf-8", "text/plain; charset=utf-8":
		if svgRoot.Match(bytes.TrimSpace(head)) {
			return "image/svg+xml", nil
		}
	}
	return ct, nil
}
