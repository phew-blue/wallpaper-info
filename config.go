package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// Config is the persisted settings. Flags override these at runtime; defaults are phew-blue.
type Config struct {
	Base      string `toml:"base"`      // background image path ("" = current wallpaper)
	Name      string `toml:"name"`      // centered label ("" = hostname, "-" = hide)
	Font      string `toml:"font"`      // family name or .ttf path ("" = Open Sans/Segoe UI)
	Accent    string `toml:"accent"`    // hostname colour
	Secondary string `toml:"secondary"` // detail-line colour
	Watch     int    `toml:"watch"`     // refresh every N minutes (0 = once)
}

func DefaultConfig() Config {
	return Config{Accent: "#0092CA", Secondary: "#6A7078"}
}

// DefaultConfigPath: %APPDATA%\wallpaper-info\config.toml on Windows, else ~/.config/wallpaper-info/config.toml.
func DefaultConfigPath() string {
	if runtime.GOOS == "windows" {
		if ad := os.Getenv("APPDATA"); ad != "" {
			return filepath.Join(ad, "wallpaper-info", "config.toml")
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
