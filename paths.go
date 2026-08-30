package main

import (
	"os"
	"path/filepath"
)

// vendorDir is the folder every Phew Blue app groups its files under. See
// notes/windows-app-layout.md.
const vendorDir = "Phew Blue"

// appDir is this app's folder inside it.
const appDir = "wallpaper-info"

// DataDir is %LOCALAPPDATA%\Phew Blue\wallpaper-info: rendered wallpapers, the
// asset cache and the manifest cache. Off Windows it falls back to ~/.cache,
// which also lets tests redirect it by setting LOCALAPPDATA.
//
// Installs made before the Phew Blue folder existed keep their data one level
// up. The first call moves it; if the move fails we keep using the old
// directory rather than silently losing a machine's cached assets.
func DataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	newDir := filepath.Join(base, vendorDir, appDir)
	oldDir := filepath.Join(base, appDir)

	if _, err := os.Stat(newDir); err == nil {
		return newDir
	}
	if _, err := os.Stat(oldDir); err == nil {
		if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err == nil {
			if err := os.Rename(oldDir, newDir); err == nil {
				return newDir
			}
		}
		return oldDir
	}
	return newDir
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
