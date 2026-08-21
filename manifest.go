package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ManifestSchema is the only manifest format this build understands. A manifest declaring
// anything else is ignored in full — partially applying an unknown format could blank a desktop.
const ManifestSchema = 1

// DefaultManifestURL is the published preset catalogue. It is a static file, so it stays
// available even when everything else in the cluster is down.
const DefaultManifestURL = "https://phew.blue/software/wallpaper-info/manifest.json"

// Asset is a downloadable release artifact.
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Background is one resolution variant of a preset's wallpaper.
type Background struct {
	W      int    `json:"w"`
	H      int    `json:"h"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// Preset is a named look: colours, font, label rule, layout, and background choices.
type Preset struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Accent      string       `json:"accent"`
	Secondary   string       `json:"secondary"`
	Font        string       `json:"font"`
	Label       string       `json:"label"` // "hostname" | "none" | a literal string
	Layout      Layout       `json:"layout"`
	Backgrounds []Background `json:"backgrounds"`
}

// Latest advertises the newest release, so the tray and installer can self-update.
type Latest struct {
	Version string `json:"version"`
	Exe     Asset  `json:"exe"`
	Setup   Asset  `json:"setup"`
}

// Manifest is the whole published catalogue.
type Manifest struct {
	Schema  int      `json:"schema"`
	Latest  Latest   `json:"latest"`
	Presets []Preset `json:"presets"`
}

// ParseManifest decodes and validates the manifest. An unknown schema is an error, not a
// partial success: a future format must be ignored wholesale rather than half-applied.
func ParseManifest(b []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, err
	}
	if m.Schema != ManifestSchema {
		return Manifest{}, fmt.Errorf("manifest schema %d not supported (want %d)", m.Schema, ManifestSchema)
	}
	return m, nil
}

// Preset looks up a preset by id.
func (m Manifest) Preset(id string) (Preset, bool) {
	for _, p := range m.Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// ManifestCachePath is where the last good manifest is kept between runs.
func ManifestCachePath() string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "wallpaper-info", "manifest.json")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "wallpaper-info", "manifest.json")
}

// ManifestFetcher resolves the manifest, preferring a fresh cache, then the network, then a
// stale cache. Callers must treat any error as "carry on with local config" — never as a
// reason to fail a render.
type ManifestFetcher struct {
	URL       string
	CachePath string
	TTL       time.Duration
	Client    *http.Client
	Now       func() time.Time
}

// NewManifestFetcher builds a fetcher with the production defaults.
func NewManifestFetcher(url string) ManifestFetcher {
	if url == "" {
		url = DefaultManifestURL
	}
	return ManifestFetcher{
		URL:       url,
		CachePath: ManifestCachePath(),
		TTL:       24 * time.Hour,
		Client:    &http.Client{Timeout: 10 * time.Second},
		Now:       time.Now,
	}
}

// Get returns the manifest from the freshest usable source.
func (f ManifestFetcher) Get() (Manifest, error) {
	if b, mod, err := f.readCache(); err == nil && f.Now().Sub(mod) < f.TTL {
		if m, err := ParseManifest(b); err == nil {
			return m, nil
		}
	}

	if b, err := f.fetch(); err == nil {
		if m, err := ParseManifest(b); err == nil {
			f.writeCache(b) // best-effort: a read-only cache dir must not fail the fetch
			return m, nil
		}
	}

	// Network failed or served something unusable: any cached manifest beats nothing.
	if b, _, err := f.readCache(); err == nil {
		if m, err := ParseManifest(b); err == nil {
			return m, nil
		}
	}
	return Manifest{}, fmt.Errorf("manifest unavailable: no usable network response or cache")
}

func (f ManifestFetcher) fetch() ([]byte, error) {
	resp, err := f.Client.Get(f.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func (f ManifestFetcher) readCache() ([]byte, time.Time, error) {
	st, err := os.Stat(f.CachePath)
	if err != nil {
		return nil, time.Time{}, err
	}
	b, err := os.ReadFile(f.CachePath)
	return b, st.ModTime(), err
}

func (f ManifestFetcher) writeCache(b []byte) {
	if err := os.MkdirAll(filepath.Dir(f.CachePath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(f.CachePath, b, 0o644)
}
