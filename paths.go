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
// An install predating the Phew Blue parent keeps its directory, and nothing is
// moved. For an upgraded install this directory IS the install directory -- it
// holds the running exe, the catalogue and the uninstaller -- so renaming it
// races the installer, which reinstalls into the path it recorded. An earlier
// version of this function did rename, and the race left LT-01 split: fonts and
// wallpaper.png under the new parent, exe, catalogue and manifest still under
// the old one, with the config pointing at both.
//
// Preferring the old directory whenever it exists resolves that state correctly
// too: the old one is the live install, so a machine caught mid-split converges
// back onto it. Fresh installs, having no old directory, get the new layout.
func DataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	if oldDir := filepath.Join(base, appDir); isDir(oldDir) {
		return oldDir
	}
	return filepath.Join(base, vendorDir, appDir)
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
