package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// PickBackground returns the background whose width is nearest the screen's, matching the
// nearest-resolution rule the provisioning script has always used.
func PickBackground(bgs []Background, screenW int) (Background, bool) {
	if len(bgs) == 0 {
		return Background{}, false
	}
	best, bestDelta := bgs[0], abs(bgs[0].W-screenW)
	for _, b := range bgs[1:] {
		if d := abs(b.W - screenW); d < bestDelta {
			best, bestDelta = b, d
		}
	}
	return best, true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ApplyPreset fills cfg from p, skipping any field the user set explicitly. Precedence is
// defaults < preset < config file < explicit flags; explicit holds the flag names seen by
// flag.Visit, so a hand-tuned machine is never silently restyled.
func ApplyPreset(cfg Config, p Preset, explicit map[string]bool) Config {
	unset := func(name string) bool { return !explicit[name] }

	if unset("accent") && p.Accent != "" {
		cfg.Accent = p.Accent
	}
	if unset("secondary") && p.Secondary != "" {
		cfg.Secondary = p.Secondary
	}
	if unset("font") && p.Font != "" {
		cfg.Font = p.Font
	}
	if unset("name") {
		switch p.Label {
		case "hostname":
			cfg.Name = "" // "" already means "use the hostname"
		case "none":
			cfg.Name = "-" // "-" hides the label
		case "":
			// preset says nothing about the label; leave cfg.Name alone
		default:
			cfg.Name = p.Label
		}
	}
	if cfg.Layout.Corner == "" && p.Layout.Corner != "" {
		cfg.Layout.Corner = p.Layout.Corner
	}
	if len(cfg.Layout.Rows) == 0 && len(p.Layout.Rows) > 0 {
		cfg.Layout.Rows = p.Layout.Rows
	}
	if p.ID != "" {
		cfg.Preset = p.ID
	}
	return cfg
}

// BackgroundCachePath is content-addressed, so a changed preset naturally re-downloads and
// superseded files can be deleted without bookkeeping.
func BackgroundCachePath(sha string) string {
	// LOCALAPPDATA is the Windows location and is honoured wherever it is set, which also lets
	// tests redirect the cache into a temp dir.
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "wallpaper-info", "backgrounds", sha+".png")
}

// VerifySHA256 reports whether the file at path hashes to want.
func VerifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, want)
	}
	return nil
}

// EnsureBackground returns a local path for bg, downloading it if the cache misses. A download
// that fails verification is deleted rather than cached, so a bad artifact cannot stick.
func EnsureBackground(bg Background, client *http.Client) (string, error) {
	if bg.URL == "" || bg.SHA256 == "" {
		return "", fmt.Errorf("background has no url or sha256")
	}
	path := BackgroundCachePath(bg.SHA256)
	if err := VerifySHA256(path, bg.SHA256); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	resp, err := client.Get(bg.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("background: HTTP %d", resp.StatusCode)
	}

	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 64<<20)); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()

	if err := VerifySHA256(tmp, bg.SHA256); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}
