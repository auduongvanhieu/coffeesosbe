package storage

import (
	"bytes"
	"io"
	"testing"
)

type memFile struct{ *bytes.Reader }

func (memFile) Close() error { return nil }

func newMemFile(b []byte) memFile { return memFile{bytes.NewReader(b)} }

func TestSniff(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	svg := []byte(`<?xml version="1.0"?>` + "\n" + `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"></svg>`)
	html := []byte(`<!doctype html><html><script>alert(1)</script></html>`)

	cases := []struct {
		name, want string
		data       []byte
	}{
		{"png", "image/png", png},
		{"svg", "image/svg+xml", svg},
		{"html", "text/html; charset=utf-8", html},
	}
	for _, tc := range cases {
		f := newMemFile(tc.data)
		got, err := sniff(f)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
		// file must be rewound for the upload that follows
		rest, _ := io.ReadAll(f)
		if !bytes.Equal(rest, tc.data) {
			t.Errorf("%s: file not rewound", tc.name)
		}
	}
}

func TestAllowedType(t *testing.T) {
	if _, ok := AllowedType(KindMenuItem, "image/svg+xml"); ok {
		t.Error("svg must not be allowed for menu items")
	}
	if ext, ok := AllowedType(KindBrandLogo, "image/svg+xml"); !ok || ext != "svg" {
		t.Error("svg must be allowed for logos")
	}
	if ext, ok := AllowedType(KindMenuItem, "image/jpeg"); !ok || ext != "jpg" {
		t.Error("jpeg must be allowed")
	}
	if _, ok := AllowedType(KindMenuItem, "text/html; charset=utf-8"); ok {
		t.Error("html must be rejected")
	}
}

func TestKeyFromURL(t *testing.T) {
	c := New(Config{PublicBaseURL: "https://cdn.example.com/"})
	if got := c.KeyFromURL("https://cdn.example.com/brands/x/menu-items/a.jpg"); got != "brands/x/menu-items/a.jpg" {
		t.Errorf("got %q", got)
	}
	if got := c.KeyFromURL("https://other.example.com/a.jpg"); got != "" {
		t.Errorf("foreign url should give empty key, got %q", got)
	}
}
