package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildManifestShape(t *testing.T) {
	presets := []presetSource{{
		ID: "phew-blue", Name: "phew-blue (default)", Description: "d",
		Accent: "#0092CA", Secondary: "#6A7078", Font: "Open Sans", Label: "hostname",
		Layout:      layoutSource{Corner: "bottom-right", Rows: []string{"os", "wan"}},
		Backgrounds: []string{"3840x2160", "1920x1080"},
	}}

	out, err := buildManifest(presets, "v0.4.0",
		map[string]asset{
			"exe":   {URL: "https://e/exe", SHA256: "aa", Size: 1},
			"setup": {URL: "https://e/setup", SHA256: "bb", Size: 2},
		},
		func(presetID, size string) (string, string) {
			return "https://wallpaper-info.phew.blue/backgrounds/" + presetID + "/" + size + ".png", "cc"
		})
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["schema"].(float64) != 1 {
		t.Errorf("schema = %v, want 1", m["schema"])
	}

	s := string(out)
	for _, want := range []string{`"id": "phew-blue"`, `"w": 3840`, `"version": "v0.4.0"`, `"sha256": "cc"`} {
		if !strings.Contains(s, want) {
			t.Errorf("manifest missing %s\n%s", want, s)
		}
	}
}

func TestBuildManifestRejectsBadBackgroundSize(t *testing.T) {
	presets := []presetSource{{ID: "x", Backgrounds: []string{"not-a-size"}}}
	if _, err := buildManifest(presets, "v1", nil, func(string, string) (string, string) { return "", "" }); err == nil {
		t.Error("want an error for a malformed background size")
	}
}
