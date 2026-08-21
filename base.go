package main

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// LoadBase returns the background to composite onto: an explicit path, else the current desktop
// wallpaper, else a solid brand-dark canvas so the tool always produces something.
func LoadBase(path string) (image.Image, error) {
	if path == "" {
		path = currentWallpaperPath() // platform-specific (empty on non-Windows)
	}
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
