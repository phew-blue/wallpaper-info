package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ManifestSchema is the only manifest format this build understands. A manifest declaring
// anything else is ignored in full — partially applying an unknown format could blank a desktop.
const ManifestSchema = 1

// DefaultManifestURL is the published preset catalogue: an asset on the newest GitHub release,
// so the URL always tracks the latest version and nothing has to be hosted separately.
const DefaultManifestURL = "https://github.com/phew-blue/wallpaper-info/releases/latest/download/manifest.json"

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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Accent      string `json:"accent"`
	Secondary   string `json:"secondary"`
	Font        string `json:"font"`
	Label       string `json:"label"` // "hostname" | "none" | a literal string
	Layout      Layout `json:"layout"`
	// FontAsset ships the preset's font so the panel renders identically on machines that
	// do not have it installed. Optional: without it, Font is treated as a family name.
	FontAsset   *Asset       `json:"font_asset,omitempty"`
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

	// Base is where this manifest came from — a directory for a local file, or the manifest
	// URL's parent for a remote one. Relative asset references resolve against it, which is
	// what makes a USB stick portable across drive letters. Not part of the JSON.
	Base string `json:"-"`
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
	return filepath.Join(DataDir(), "manifest.json")
}

// isLocalSource reports whether a manifest or asset reference points at the filesystem
// rather than the network. Removable media (a provisioning USB) and air-gapped OB trucks
// use local manifests, so http(s) is the exception, not the rule.
func isLocalSource(s string) bool {
	if s == "" {
		return false
	}
	return !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://")
}

// LocalPath strips a file:// prefix, returning a plain filesystem path. A Windows file URL is
// file:///C:/dir, so the leading slash is dropped only when a drive letter follows it —
// stripping it unconditionally would turn the Unix path /tmp/x into the relative tmp/x.
func LocalPath(s string) string {
	p := strings.TrimPrefix(s, "file://")
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return p
}

// ResolveAssetURL turns an asset reference into something fetchable. Absolute http(s)
// references are returned untouched; anything else is resolved against the manifest's own
// location. That is what lets a USB stick be portable: the manifest says
// "backgrounds/x.png" and it works whether the stick mounts as E: or F:.
func ResolveAssetURL(base, ref string) string {
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if base == "" {
		return ref
	}
	if isLocalSource(base) {
		return filepath.Join(base, filepath.FromSlash(ref))
	}
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(ref, "/")
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
	// A local manifest (USB stick, air-gapped machine) is read straight from disk. There is
	// nothing to cache — the file is the source — and no network failure to fall back from, so
	// a bad local manifest is reported rather than silently swallowed.
	if isLocalSource(f.URL) {
		path := LocalPath(f.URL)
		b, err := os.ReadFile(path)
		if err != nil {
			return Manifest{}, err
		}
		m, err := ParseManifest(b)
		if err != nil {
			return Manifest{}, err
		}
		m.Base = filepath.Dir(path)
		return m, nil
	}

	if b, mod, err := f.readCache(); err == nil && f.Now().Sub(mod) < f.TTL {
		if m, err := ParseManifest(b); err == nil {
			m.Base = remoteBase(f.URL)
			return m, nil
		}
	}

	if b, err := f.fetch(); err == nil {
		if m, err := ParseManifest(b); err == nil {
			f.writeCache(b) // best-effort: a read-only cache dir must not fail the fetch
			m.Base = remoteBase(f.URL)
			return m, nil
		}
	}

	// Network failed or served something unusable: any cached manifest beats nothing.
	if b, _, err := f.readCache(); err == nil {
		if m, err := ParseManifest(b); err == nil {
			m.Base = remoteBase(f.URL)
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

// remoteBase is the directory part of a manifest URL, so a remote manifest can also use
// relative asset references.
func remoteBase(u string) string {
	if i := strings.LastIndex(u, "/"); i > 0 {
		return u[:i]
	}
	return ""
}
