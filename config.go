package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// Layout controls which rows appear and where the panel sits. Zero values mean "unset", so a
// preset can supply them without overriding an explicit local choice.
type Layout struct {
	Corner string   `toml:"corner"` // bottom-right (default) | bottom-left | top-right | top-left
	Rows   []string `toml:"rows"`   // ordered subset of user, os, uptime, cpu, ram, disk, nics, wan
}

// Config is the persisted settings. Flags override these at runtime; defaults are phew-blue.
// Precedence, lowest to highest: defaults < preset < this file < explicit flags.
type Config struct {
	Base      string `toml:"base"`      // background image path ("" = current wallpaper)
	Name      string `toml:"name"`      // centered label ("" = hostname, "-" = hide)
	Font      string `toml:"font"`      // family name or .ttf path ("" = Open Sans/Segoe UI)
	Accent    string `toml:"accent"`    // hostname colour
	Secondary string `toml:"secondary"` // detail-line colour
	Watch     int    `toml:"watch"`     // refresh every N minutes (0 = once)
	Preset    string `toml:"preset"`    // id of the last applied preset ("" = purely local config)
	Manifest  string `toml:"manifest"`  // preset catalogue: URL or local path ("" = the published one)
	Layout    Layout `toml:"layout"`
}

func DefaultConfig() Config {
	return Config{Accent: "#0092CA", Secondary: "#6A7078"}
}

// DefaultConfigPath: %APPDATA%\Phew Blue\wallpaper-info\config.toml on Windows,
// else ~/.config/wallpaper-info/config.toml. Roaming rather than local, so a
// user's chosen look follows them between machines.
//
// A config written before the Phew Blue folder existed sits one level up; it is
// still honoured, so an upgrade does not silently reset someone's wallpaper.
func DefaultConfigPath() string {
	if runtime.GOOS == "windows" {
		if ad := os.Getenv("APPDATA"); ad != "" {
			newPath := filepath.Join(ad, vendorDir, appDir, "config.toml")
			if _, err := os.Stat(newPath); err == nil {
				return newPath
			}
			if old := filepath.Join(ad, appDir, "config.toml"); fileExists(old) {
				return old
			}
			return newPath
		}
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "wallpaper-info", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "wallpaper-info", "config.toml")
}

// LoadConfig returns defaults merged with the file at path (if it exists).
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	if _, err := os.Stat(path); err != nil {
		return cfg, nil // no file: just defaults
	}
	_, err := toml.DecodeFile(path, &cfg)
	return cfg, err
}

// SaveConfig writes cfg to path (creating the directory), with a header comment.
func SaveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString("# wallpaper-info config. Flags override these. Colours are hex.\n"); err != nil {
		return err
	}
	return toml.NewEncoder(f).Encode(cfg)
}
