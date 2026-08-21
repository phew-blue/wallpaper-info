//go:build !windows

package main

import (
	"errors"
	"image"
)

func currentWallpaperPath() string { return "" }

// SetWallpaper is Windows-only for now; use --out to render a PNG on other platforms.
func SetWallpaper(img image.Image) error {
	return errors.New("setting the wallpaper is only supported on Windows; use --out to write a PNG")
}

// OutputPath: nothing is written off Windows, so there is no output file to guard against.
func OutputPath() string { return "" }
