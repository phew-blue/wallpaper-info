package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const sampleManifest = `{
  "schema": 1,
  "latest": {
    "version": "v0.4.0",
    "exe":   {"url": "https://example.invalid/wpi.exe",   "sha256": "aa", "size": 10},
    "setup": {"url": "https://example.invalid/setup.exe", "sha256": "bb", "size": 20}
  },
  "presets": [{
    "id": "phew-blue",
    "name": "phew-blue (default)",
    "accent": "#0092CA",
    "secondary": "#6A7078",
    "label": "hostname",
    "layout": {"corner": "bottom-right", "rows": ["os", "wan"]},
    "backgrounds": [
      {"w": 3840, "h": 2160, "url": "https://example.invalid/4k.png", "sha256": "cc"},
      {"w": 1920, "h": 1080, "url": "https://example.invalid/hd.png", "sha256": "dd"}
    ]
  }]
}`

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte(sampleManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.Latest.Version != "v0.4.0" {
		t.Errorf("Version = %q", m.Latest.Version)
	}
	p, ok := m.Preset("phew-blue")
	if !ok {
		t.Fatal("preset phew-blue not found")
	}
	if p.Accent != "#0092CA" || p.Layout.Corner != "bottom-right" || len(p.Backgrounds) != 2 {
		t.Errorf("preset parsed wrong: %+v", p)
	}
	if _, ok := m.Preset("nope"); ok {
		t.Error("unknown preset id reported as found")
	}
}

func TestParseManifestRejectsUnknownSchema(t *testing.T) {
	if _, err := ParseManifest([]byte(`{"schema": 99, "presets": []}`)); err == nil {
		t.Error("want an error for an unknown schema; a future format must be ignored wholesale")
	}
}

func TestParseManifestRejectsMalformed(t *testing.T) {
	if _, err := ParseManifest([]byte(`{not json`)); err == nil {
		t.Error("want an error for malformed JSON")
	}
}

func TestFetcherUsesFreshCacheWithoutNetwork(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(cache, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("network was used despite a fresh cache")
	}))
	defer srv.Close()

	f := ManifestFetcher{URL: srv.URL, CachePath: cache, TTL: time.Hour, Client: srv.Client(), Now: time.Now}
	if _, err := f.Get(); err != nil {
		t.Fatal(err)
	}
}

func TestFetcherFallsBackToStaleCacheWhenOffline(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(cache, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(cache, old, old); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := ManifestFetcher{URL: srv.URL, CachePath: cache, TTL: time.Hour, Client: srv.Client(), Now: time.Now}
	m, err := f.Get()
	if err != nil {
		t.Fatalf("stale cache should rescue an offline fetch: %v", err)
	}
	if m.Latest.Version != "v0.4.0" {
		t.Errorf("got %q from stale cache", m.Latest.Version)
	}
}

func TestFetcherWritesCacheOnSuccess(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "manifest.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleManifest))
	}))
	defer srv.Close()

	f := ManifestFetcher{URL: srv.URL, CachePath: cache, TTL: time.Hour, Client: srv.Client(), Now: time.Now}
	if _, err := f.Get(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Errorf("cache not written: %v", err)
	}
}

func TestFetcherErrorsWhenNoCacheAndNoNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := ManifestFetcher{
		URL: srv.URL, CachePath: filepath.Join(t.TempDir(), "absent.json"),
		TTL: time.Hour, Client: srv.Client(), Now: time.Now,
	}
	if _, err := f.Get(); err == nil {
		t.Error("want an error when there is neither cache nor network")
	}
}
