package main

import (
	"fmt"
	"image/png"
	"net/http"
	"os"
	"time"
)

// App is the run-time state that the one-shot run, the watch loop, and the tray all drive.
// It exists so the tray can trigger a render or a preset switch without reaching into main's
// local variables.
type App struct {
	Cfg      Config
	CfgPath  string
	Demo     bool   // render synthetic facts (docs screenshots)
	Out      string // write a PNG here instead of setting the wallpaper
	Explicit map[string]bool
	Fetcher  ManifestFetcher

	lastSig string
}

// RenderOnce renders and sets the wallpaper. force skips the unchanged-signature check, which
// is what the tray's "Refresh now" and a preset switch need.
func (a *App) RenderOnce(force bool) error {
	info := Gather()
	if a.Demo {
		info = DemoInfo()
	}
	sig := info.SigWith(a.Cfg)
	if !force && a.Out == "" && sig == a.lastSig {
		return nil // nothing changed: skip the render + wallpaper set
	}

	label := a.Cfg.Name
	switch label {
	case "":
		label = info.Host // default: the hostname
	case "-":
		label = "" // explicitly hidden
	}

	bg, err := LoadBase(a.Cfg.Base)
	if err != nil {
		return err
	}
	// An unknown corner falls back to the default rather than moving the panel somewhere odd.
	corner, _ := ParseCorner(a.Cfg.Layout.Corner)
	img := Render(bg, info, RenderOpts{
		Label:     label,
		Font:      a.Cfg.Font,
		Accent:    a.Cfg.Accent,
		Secondary: a.Cfg.Secondary,
		Corner:    corner,
		Rows:      a.Cfg.Layout.Rows,
	})

	if a.Out != "" {
		f, err := os.Create(a.Out)
		if err != nil {
			return err
		}
		defer f.Close()
		return png.Encode(f, img)
	}
	if err := SetWallpaper(img); err != nil {
		return err
	}
	a.lastSig = sig
	return nil
}

// ApplyPresetByID switches preset at run time, persists the choice, and re-renders.
func (a *App) ApplyPresetByID(id string) error {
	m, err := a.Fetcher.Get()
	if err != nil {
		return err
	}
	p, ok := m.Preset(id)
	if !ok {
		return fmt.Errorf("preset %q not found", id)
	}

	a.Cfg = ApplyPreset(a.Cfg, p, a.Explicit)
	client := &http.Client{Timeout: 60 * time.Second}
	if bg, ok := PickBackground(p.Backgrounds, ScreenWidth()); ok {
		path, err := EnsureBackground(bg, m.Base, client)
		if err != nil {
			// A missing background is not fatal: LoadBase falls back to the current wallpaper.
			fmt.Fprintln(os.Stderr, "wallpaper-info: background:", err)
		} else {
			a.Cfg.Base = path
		}
	}
	// Ship-the-font, so switching preset from the tray looks the same on any machine.
	if p.FontAsset != nil {
		path, err := EnsureFont(*p.FontAsset, m.Base, client)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info: font:", err)
		} else {
			a.Cfg.Font = path
		}
	}
	if err := SaveConfig(a.CfgPath, a.Cfg); err != nil {
		return err
	}
	return a.RenderOnce(true)
}

// Watch runs the refresh loop until the process exits. It is shared by --watch and --tray.
func (a *App) Watch() {
	for a.Cfg.Watch > 0 {
		time.Sleep(time.Duration(a.Cfg.Watch) * time.Minute)
		if err := a.RenderOnce(false); err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
		}
	}
}
