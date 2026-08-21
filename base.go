package main

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// LoadBase returns the background to composite onto: an explicit path, else the current desktop
// wallpaper, else a solid brand-dark canvas so the tool always produces something.
func LoadBase(path string) (image.Image, error) {
	path = baseCandidate(path, currentWallpaperPath(), OutputPath())
	if path != "" {
		if f, err := os.Open(path); err == nil {
			defer f.Close()
			if img, _, err := image.Decode(f); err == nil {
				return img, nil
			}
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{0x23, 0x26, 0x2B, 0xff}), image.Point{}, draw.Src)
	return img, nil
}

// baseCandidate decides which file to composite onto. An explicit path always wins. The
// current desktop wallpaper is used only when it is not our own previous render —
// compositing onto that would stack the info panel on itself at every refresh, which is
// why the old provisioning shortcut had to pin an explicit --base.
func baseCandidate(explicit, current, output string) string {
	if explicit != "" {
		return explicit
	}
	if output != "" && samePath(current, output) {
		return ""
	}
	return current
}

// samePath compares two paths case-insensitively, matching Windows filesystem semantics.
func samePath(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
