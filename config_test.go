package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsPresetAndLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
preset = "phew-blue"
accent = "#112233"

[layout]
corner = "top-left"
rows = ["os", "wan"]
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Preset != "phew-blue" {
		t.Errorf("Preset = %q, want phew-blue", cfg.Preset)
	}
	if cfg.Accent != "#112233" {
		t.Errorf("Accent = %q, want the file value to beat the default", cfg.Accent)
	}
	if cfg.Layout.Corner != "top-left" {
		t.Errorf("Layout.Corner = %q, want top-left", cfg.Layout.Corner)
	}
	if len(cfg.Layout.Rows) != 2 || cfg.Layout.Rows[0] != "os" || cfg.Layout.Rows[1] != "wan" {
		t.Errorf("Layout.Rows = %v, want [os wan]", cfg.Layout.Rows)
	}
}

func TestLoadConfigMissingFileKeepsBrandDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Accent != "#0092CA" || cfg.Secondary != "#6A7078" {
		t.Errorf("defaults = %q/%q, want #0092CA/#6A7078", cfg.Accent, cfg.Secondary)
	}
	if cfg.Preset != "" || cfg.Layout.Corner != "" || cfg.Layout.Rows != nil {
		t.Error("absent config should leave preset/layout zero so a preset can fill them")
	}
}

func TestSaveConfigRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := DefaultConfig()
	want.Preset = "phew-blue"
	want.Layout = Layout{Corner: "bottom-left", Rows: []string{"user", "wan"}}

	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset != want.Preset || got.Layout.Corner != want.Layout.Corner || len(got.Layout.Rows) != 2 {
		t.Errorf("round-trip lost data: got %+v, want %+v", got, want)
	}
}
