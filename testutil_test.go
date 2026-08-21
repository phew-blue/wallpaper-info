package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// mustPNG returns the bytes of a blank w×h PNG.
func mustPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
