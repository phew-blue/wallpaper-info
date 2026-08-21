// Command manifest turns presets/*.toml plus the release artifacts into the manifest.json
// published at https://phew.blue/software/wallpaper-info. It runs in the release workflow after the
// GitHub release assets exist, so the manifest can never advertise a version that failed to
// upload.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

type layoutSource struct {
	Corner string   `toml:"corner"`
	Rows   []string `toml:"rows"`
}

// presetSource is the on-disk authoring format. Backgrounds are "WxH" strings; the generator
// resolves each to a published URL plus the sha256 of the actual PNG.
type presetSource struct {
	ID          string       `toml:"id"`
	Name        string       `toml:"name"`
	Description string       `toml:"description"`
	Accent      string       `toml:"accent"`
	Secondary   string       `toml:"secondary"`
	Font        string       `toml:"font"`
	Label       string       `toml:"label"`
	Layout      layoutSource `toml:"layout"`
	Backgrounds []string     `toml:"backgrounds"`
}

type asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type background struct {
	W      int    `json:"w"`
	H      int    `json:"h"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type layout struct {
	Corner string   `json:"corner"`
	Rows   []string `json:"rows"`
}

type preset struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Accent      string       `json:"accent"`
	Secondary   string       `json:"secondary"`
	Font        string       `json:"font"`
	Label       string       `json:"label"`
	Layout      layout       `json:"layout"`
	Backgrounds []background `json:"backgrounds"`
}

type latest struct {
	Version string `json:"version"`
	Exe     asset  `json:"exe"`
	Setup   asset  `json:"setup"`
}

type manifest struct {
	Schema  int      `json:"schema"`
	Latest  latest   `json:"latest"`
	Presets []preset `json:"presets"`
}

// resolveBG returns the public URL and sha256 for one preset background.
type resolveBG func(presetID, size string) (url, sha string)

// buildManifest renders the published JSON. It is separated from I/O so it can be tested.
func buildManifest(presets []presetSource, version string, assets map[string]asset, resolve resolveBG) ([]byte, error) {
	m := manifest{Schema: 1, Latest: latest{Version: version, Exe: assets["exe"], Setup: assets["setup"]}}

	for _, ps := range presets {
		p := preset{
			ID: ps.ID, Name: ps.Name, Description: ps.Description,
			Accent: ps.Accent, Secondary: ps.Secondary, Font: ps.Font, Label: ps.Label,
			Layout: layout{Corner: ps.Layout.Corner, Rows: ps.Layout.Rows},
		}
		for _, size := range ps.Backgrounds {
			var w, h int
			if _, err := fmt.Sscanf(size, "%dx%d", &w, &h); err != nil || w <= 0 || h <= 0 {
				return nil, fmt.Errorf("preset %q: malformed background size %q (want WxH)", ps.ID, size)
			}
			url, sha := resolve(ps.ID, size)
			p.Backgrounds = append(p.Backgrounds, background{W: w, H: h, URL: url, SHA256: sha})
		}
		m.Presets = append(m.Presets, p)
	}
	return json.MarshalIndent(m, "", "  ")
}

func fileAsset(path, url string) (asset, error) {
	if path == "" {
		return asset{}, nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return asset{}, err
	}
	sha, err := fileSHA256(path)
	if err != nil {
		return asset{}, err
	}
	return asset{URL: url, SHA256: sha, Size: st.Size()}, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func loadPresets(dir string) ([]presetSource, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths) // stable output, so an unchanged release produces an unchanged manifest
	var out []presetSource
	for _, p := range paths {
		var ps presetSource
		if _, err := toml.DecodeFile(p, &ps); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if ps.ID == "" {
			return nil, fmt.Errorf("%s: preset has no id", p)
		}
		out = append(out, ps)
	}
	return out, nil
}

func main() {
	presetsDir := flag.String("presets", "presets", "directory of preset .toml files")
	bgDir := flag.String("backgrounds", "", "directory of background PNGs (<preset>/<WxH>.png) for hashing")
	version := flag.String("version", "", "release version, e.g. v0.4.0")
	exePath := flag.String("exe", "", "path to the built .exe, for size and sha256")
	setupPath := flag.String("setup", "", "path to the built installer, for size and sha256")
	repo := flag.String("repo", "phew-blue/wallpaper-info", "GitHub repo hosting the release assets")
	baseURL := flag.String("base-url", "https://phew.blue/software/wallpaper-info", "public base URL for backgrounds")
	out := flag.String("out", "", "write here instead of stdout")
	flag.Parse()

	presets, err := loadPresets(*presetsDir)
	if err != nil {
		fatal(err)
	}

	rel := fmt.Sprintf("https://github.com/%s/releases/download/%s", *repo, *version)
	exe, err := fileAsset(*exePath, rel+"/wallpaper-info-windows-amd64.exe")
	if err != nil {
		fatal(err)
	}
	setup, err := fileAsset(*setupPath, rel+"/"+filepath.Base(*setupPath))
	if err != nil {
		fatal(err)
	}

	resolve := func(presetID, size string) (string, string) {
		url := fmt.Sprintf("%s/backgrounds/%s/%s.png", *baseURL, presetID, size)
		if *bgDir == "" {
			return url, ""
		}
		sha, err := fileSHA256(filepath.Join(*bgDir, presetID, size+".png"))
		if err != nil {
			// A missing background is not fatal: the client falls back to the current
			// wallpaper, and a release should not be blocked on one missing image.
			fmt.Fprintf(os.Stderr, "manifest: warning: %s/%s: %v\n", presetID, size, err)
			return url, ""
		}
		return url, sha
	}

	b, err := buildManifest(presets, *version, map[string]asset{"exe": exe, "setup": setup}, resolve)
	if err != nil {
		fatal(err)
	}
	b = append(b, '\n')

	if *out == "" {
		os.Stdout.Write(b)
		return
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "manifest:", err)
	os.Exit(1)
}
