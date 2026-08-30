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

// cachePath is content-addressed, so changed assets naturally re-fetch and superseded files can
// be deleted without bookkeeping.
func cachePath(kind, sha, ext string) string {
	return filepath.Join(DataDir(), kind, sha+ext)
}

// BackgroundCachePath is where a preset's wallpaper image is kept.
func BackgroundCachePath(sha string) string { return cachePath("backgrounds", sha, ".png") }

// FontCachePath is where a preset's font file is kept. Fonts are cached rather than required to
// be installed: the panel must render identically on a machine that has never seen the brand
// fonts, and the wallpaper keeps re-rendering long after any USB stick is gone.
func FontCachePath(sha string) string { return cachePath("fonts", sha, ".ttf") }

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

// EnsureBackground returns a local path for bg, fetching it if the cache misses. base is the
// manifest's own location, so relative references work (see ResolveAssetURL). The image is
// always copied into the content-addressed cache, including from a local source: a USB stick
// gets unplugged, and the wallpaper has to keep rendering afterwards.
//
// Anything that fails verification is deleted rather than cached, so a bad artifact cannot stick.
func EnsureBackground(bg Background, base string, client *http.Client) (string, error) {
	return ensureCached(bg.URL, bg.SHA256, BackgroundCachePath(bg.SHA256), base, client)
}

// EnsureFont returns a local path for a preset's font file, fetching it if the cache misses.
// Shipping the font with the preset is what makes the panel look the same everywhere: a machine
// with no Open Sans installed would otherwise silently fall back to Segoe UI and render a
// visibly different wallpaper from its neighbour.
func EnsureFont(a Asset, base string, client *http.Client) (string, error) {
	return ensureCached(a.URL, a.SHA256, FontCachePath(a.SHA256), base, client)
}

// ensureCached copies an asset into the content-addressed cache, from the network or from local
// media, and verifies it. Anything failing verification is deleted rather than cached, so a bad
// artifact cannot stick. Copying (rather than referencing in place) is deliberate: a USB stick
// gets unplugged and the wallpaper has to keep rendering afterwards.
func ensureCached(ref, sha, path, base string, client *http.Client) (string, error) {
	if ref == "" || sha == "" {
		return "", fmt.Errorf("asset has no url or sha256")
	}
	if err := VerifySHA256(path, sha); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	rc, err := openAsset(ResolveAssetURL(base, ref), client)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(rc, 64<<20)); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()

	if err := VerifySHA256(tmp, sha); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// openAsset opens an asset from wherever it lives: a local file (USB stick, air-gapped share)
// or over http(s). Callers close the returned reader.
func openAsset(src string, client *http.Client) (io.ReadCloser, error) {
	if isLocalSource(src) {
		return os.Open(LocalPath(src))
	}
	resp, err := client.Get(src)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("asset: HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}
