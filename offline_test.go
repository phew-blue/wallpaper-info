package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsLocalSource(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://example.com/manifest.json", false},
		{"http://example.com/manifest.json", false},
		{`E:\phew-blue\wallpaper-info\manifest.json`, true},
		{"/mnt/usb/manifest.json", true},
		{"file:///E:/phew-blue/manifest.json", true},
		{"manifest.json", true},
		{"", false},
	} {
		if got := isLocalSource(tc.in); got != tc.want {
			t.Errorf("isLocalSource(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLocalPathHandlesFileURLs(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"file:///E:/phew-blue/manifest.json", "E:/phew-blue/manifest.json"}, // drive letter: drop the slash
		{"file:///mnt/usb/manifest.json", "/mnt/usb/manifest.json"},          // unix: keep it
		{`E:\phew-blue\manifest.json`, `E:\phew-blue\manifest.json`},
		{"/mnt/usb/manifest.json", "/mnt/usb/manifest.json"},
	} {
		if got := LocalPath(tc.in); got != tc.want {
			t.Errorf("LocalPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A USB stick is E: on one machine and F: on another, so its manifest must use relative asset
// references. Absolute http(s) references from the published manifest must be left alone.
func TestResolveAssetURL(t *testing.T) {
	for _, tc := range []struct{ name, base, ref, want string }{
		{"absolute wins over local base", "/mnt/usb/wallpaper-info", "https://example.com/a.png", "https://example.com/a.png"},
		{"relative against a remote base", "https://example.com/dl", "backgrounds/a.png", "https://example.com/dl/backgrounds/a.png"},
		{"remote base with trailing slash", "https://example.com/dl/", "backgrounds/a.png", "https://example.com/dl/backgrounds/a.png"},
		{"no base at all", "", "backgrounds/a.png", "backgrounds/a.png"},
		{"empty ref", "/mnt/usb", "", ""},
	} {
		if got := ResolveAssetURL(tc.base, tc.ref); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}

	// Local joins use the OS separator, so compare against filepath.Join rather than a literal.
	got := ResolveAssetURL("/mnt/usb/wallpaper-info", "backgrounds/a.png")
	if want := filepath.Join("/mnt/usb/wallpaper-info", "backgrounds", "a.png"); got != want {
		t.Errorf("local join: got %q, want %q", got, want)
	}
}

// The whole point of the USB: no network, and the manifest still resolves.
func TestFetcherReadsLocalManifestWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	f := ManifestFetcher{URL: path, CachePath: filepath.Join(dir, "cache.json"), TTL: time.Hour, Now: time.Now}
	m, err := f.Get() // Client is nil on purpose: touching the network would panic
	if err != nil {
		t.Fatal(err)
	}
	if m.Latest.Version != "v0.4.0" {
		t.Errorf("version = %q", m.Latest.Version)
	}
	if m.Base != dir {
		t.Errorf("Base = %q, want the manifest's own directory %q", m.Base, dir)
	}
}

func TestFetcherReportsBadLocalManifest(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(bad, []byte(`{"schema": 99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A local manifest has no network to fall back from, so the error must surface rather than
	// being swallowed as "carry on with local config".
	if _, err := (ManifestFetcher{URL: bad, Now: time.Now}).Get(); err == nil {
		t.Error("want an error for an unsupported schema in a local manifest")
	}
	if _, err := (ManifestFetcher{URL: filepath.Join(dir, "absent.json"), Now: time.Now}).Get(); err == nil {
		t.Error("want an error for a missing local manifest")
	}
}

// The stick gets unplugged, so a local background must be copied into the cache, not referenced
// in place.
func TestEnsureBackgroundCopiesFromLocalSourceIntoCache(t *testing.T) {
	usb := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())

	payload := mustPNG(t, 8, 6)
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])
	if err := os.MkdirAll(filepath.Join(usb, "backgrounds"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usb, "backgrounds", "tl.png"), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := EnsureBackground(Background{URL: "backgrounds/tl.png", SHA256: good}, usb, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(got, usb) {
		t.Errorf("returned %q, which is still on the removable media", got)
	}
	if err := VerifySHA256(got, good); err != nil {
		t.Errorf("cached copy does not verify: %v", err)
	}

	// Unplug the stick: the cached copy must still be usable.
	os.RemoveAll(usb)
	if err := VerifySHA256(got, good); err != nil {
		t.Errorf("after the media went away: %v", err)
	}
}

func TestEnsureBackgroundRejectsBadLocalHash(t *testing.T) {
	usb := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := os.WriteFile(filepath.Join(usb, "x.png"), []byte("not the right bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureBackground(Background{URL: "x.png", SHA256: "deadbeef"}, usb, nil); err == nil {
		t.Error("want an error when a local background fails verification")
	}
}
