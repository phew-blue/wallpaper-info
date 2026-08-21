package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPickBackgroundChoosesNearestWidth(t *testing.T) {
	bgs := []Background{
		{W: 3840, H: 2160, URL: "4k"},
		{W: 1920, H: 1080, URL: "hd"},
		{W: 2560, H: 1440, URL: "qhd"},
	}
	for _, tc := range []struct {
		screen int
		want   string
	}{
		{3840, "4k"}, {1920, "hd"}, {2560, "qhd"},
		{2000, "hd"},  // 80 away from hd, 560 from qhd
		{3000, "qhd"}, // 440 from qhd, 840 from 4k
		{800, "hd"},   // below everything: nearest is still hd
	} {
		got, ok := PickBackground(bgs, tc.screen)
		if !ok || got.URL != tc.want {
			t.Errorf("PickBackground(%d) = %q, want %q", tc.screen, got.URL, tc.want)
		}
	}
	if _, ok := PickBackground(nil, 1920); ok {
		t.Error("empty background list should report not-found")
	}
}

func TestApplyPresetDoesNotOverrideExplicitSettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Accent = "#FF0000" // the user set this on the command line
	p := Preset{
		ID: "phew-blue", Accent: "#0092CA", Secondary: "#6A7078", Font: "Open Sans",
		Label: "hostname", Layout: Layout{Corner: "top-left", Rows: []string{"os"}},
	}

	got := ApplyPreset(cfg, p, map[string]bool{"accent": true})

	if got.Accent != "#FF0000" {
		t.Errorf("Accent = %q, want the explicit flag to win over the preset", got.Accent)
	}
	if got.Secondary != "#6A7078" || got.Font != "Open Sans" {
		t.Errorf("preset should fill unset fields: %+v", got)
	}
	if got.Layout.Corner != "top-left" || len(got.Layout.Rows) != 1 {
		t.Errorf("preset layout not applied: %+v", got.Layout)
	}
	if got.Preset != "phew-blue" {
		t.Errorf("Preset id = %q, want it recorded", got.Preset)
	}
}

func TestApplyPresetLabelModes(t *testing.T) {
	for _, tc := range []struct{ label, want string }{
		{"hostname", ""}, // "" means "use the hostname", the existing convention
		{"none", "-"},    // "-" hides the label
		{"BUILD BOX", "BUILD BOX"},
	} {
		got := ApplyPreset(DefaultConfig(), Preset{Label: tc.label}, nil)
		if got.Name != tc.want {
			t.Errorf("label %q -> Name %q, want %q", tc.label, got.Name, tc.want)
		}
	}
}

func TestEnsureBackgroundVerifiesSHA256(t *testing.T) {
	payload := []byte("fake png bytes")
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	t.Setenv("LOCALAPPDATA", t.TempDir())

	path, err := EnsureBackground(Background{URL: srv.URL, SHA256: good}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(payload) {
		t.Error("cached file content does not match what was served")
	}

	if _, err := EnsureBackground(Background{URL: srv.URL, SHA256: "deadbeef"}, srv.Client()); err == nil {
		t.Error("want an error when the sha256 does not match")
	}
	if _, err := os.Stat(BackgroundCachePath("deadbeef")); !os.IsNotExist(err) {
		t.Error("a mismatched download must not be left in the cache")
	}
}

func TestEnsureBackgroundReusesCache(t *testing.T) {
	payload := []byte("cached bytes")
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])

	t.Setenv("LOCALAPPDATA", t.TempDir())

	cached := BackgroundCachePath(good)
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cached, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("network used despite a valid cached background")
	}))
	defer srv.Close()

	if _, err := EnsureBackground(Background{URL: srv.URL, SHA256: good}, srv.Client()); err != nil {
		t.Fatal(err)
	}
}
