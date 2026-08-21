// Command manifest turns presets/*.toml plus the release artifacts into the manifest.json
// attached to each GitHub release. Backgrounds ship as release assets too, so a release is the
// single source of truth and nothing has to be published anywhere else.
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
	// BackgroundSet names the image set to use, so colour-variant presets can share one set of
	// PNGs instead of shipping duplicates. Defaults to the preset id.
	BackgroundSet string `toml:"background_set"`
	// FontFile names a file in the fonts directory to ship with this preset, so the panel
	// renders identically on machines that do not have the family installed.
	FontFile string `toml:"font_file"`
}

// bgSet is the image set a preset draws on.
func (p presetSource) bgSet() string {
	if p.BackgroundSet != "" {
		return p.BackgroundSet
	}
	return p.ID
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
	FontAsset   *asset       `json:"font_asset,omitempty"`
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

// resolveFont returns the published URL and sha256 for a preset's font file.
type resolveFont func(file string) (url, sha string)

// buildManifest renders the published JSON. It is separated from I/O so it can be tested.
func buildManifest(presets []presetSource, version string, assets map[string]asset, resolve resolveBG, resolveF resolveFont) ([]byte, error) {
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
			url, sha := resolve(ps.bgSet(), size)
			p.Backgrounds = append(p.Backgrounds, background{W: w, H: h, URL: url, SHA256: sha})
		}
		if ps.FontFile != "" && resolveF != nil {
			if url, sha := resolveF(ps.FontFile); url != "" {
				p.FontAsset = &asset{URL: url, SHA256: sha}
			}
		}
		m.Presets = append(m.Presets, p)
	}
	return json.MarshalIndent(m, "", "  ")
}

// BackgroundAssetName is the flat release-asset filename for one background. Release assets
// have no directory structure, so the set and size are folded into the name.
func BackgroundAssetName(set, size string) string {
	return fmt.Sprintf("background-%s-%s.png", set, size)
}

// stageBackgrounds copies every background a preset references into dir under its flat asset
// name, ready for upload. Returns how many files were staged.
func stageBackgrounds(presets []presetSource, bgDir, dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	n := 0
	for _, ps := range presets {
		for _, size := range ps.Backgrounds {
			name := BackgroundAssetName(ps.bgSet(), size)
			if seen[name] {
				continue // shared image set: stage it once
			}
			seen[name] = true
			src := filepath.Join(bgDir, ps.bgSet(), size+".png")
			b, err := os.ReadFile(src)
			if err != nil {
				fmt.Fprintf(os.Stderr, "manifest: warning: %v\n", err)
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
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
	localMode := flag.Bool("local", false, "emit relative asset URLs for removable media (a USB manifest must work as E: or F:)")
	stage := flag.String("stage", "", "copy referenced backgrounds here under their flat release-asset names")
	fontDir := flag.String("fonts", "fonts", "directory of font files a preset may ship")
	stageFonts := flag.String("stage-fonts", "", "copy referenced fonts here (defaults to -stage)")
	out := flag.String("out", "", "write here instead of stdout")
	flag.Parse()

	presets, err := loadPresets(*presetsDir)
	if err != nil {
		fatal(err)
	}

	rel := fmt.Sprintf("https://github.com/%s/releases/download/%s", *repo, *version)
	if *localMode {
		// On removable media everything sits next to the manifest, so references are
		// relative: the stick works whatever drive letter it mounts as.
		rel = ""
	}
	exe, err := fileAsset(*exePath, joinRef(rel, "wallpaper-info-windows-amd64.exe"))
	if err != nil {
		fatal(err)
	}
	setup, err := fileAsset(*setupPath, joinRef(rel, filepath.Base(*setupPath)))
	if err != nil {
		fatal(err)
	}

	// Backgrounds ship as release assets alongside the binaries, so the release is the single
	// source of truth and nothing has to be published anywhere else. Release assets are flat,
	// hence the background-<set>-<WxH>.png naming.
	resolve := func(set, size string) (string, string) {
		url := joinRef(rel, BackgroundAssetName(set, size))
		if *localMode {
			url = joinRef("backgrounds", BackgroundAssetName(set, size))
		}
		if *bgDir == "" {
			return url, ""
		}
		sha, err := fileSHA256(filepath.Join(*bgDir, set, size+".png"))
		if err != nil {
			// A missing background is not fatal: the client falls back to the current
			// wallpaper, and a release should not be blocked on one missing image.
			fmt.Fprintf(os.Stderr, "manifest: warning: %s/%s: %v\n", set, size, err)
			return url, ""
		}
		return url, sha
	}

	// Fonts ship with the preset so the panel does not depend on what happens to be installed.
	// On removable media they sit in fonts\ next to the manifest; in a release they are flat
	// assets like the backgrounds.
	resolveF := func(file string) (string, string) {
		var url string
		if *localMode {
			url = joinRef("fonts", file)
		} else {
			url = joinRef(rel, "font-"+file)
		}
		if *fontDir == "" {
			return url, ""
		}
		sha, err := fileSHA256(filepath.Join(*fontDir, file))
		if err != nil {
			// Not fatal: the client falls back to a family name, then to Segoe UI.
			fmt.Fprintf(os.Stderr, "manifest: warning: font %s: %v\n", file, err)
			return url, ""
		}
		return url, sha
	}

	if *stage != "" && *bgDir != "" {
		n, err := stageBackgrounds(presets, *bgDir, *stage)
		if err != nil {
			fatal(err)
		}
		fmt.Fprintf(os.Stderr, "manifest: staged %d background(s) in %s\n", n, *stage)
	}

	if *fontDir != "" {
		dest := *stageFonts
		if dest == "" {
			dest = *stage
		}
		if dest != "" {
			n, err := stageFontFiles(presets, *fontDir, dest, *localMode)
			if err != nil {
				fatal(err)
			}
			fmt.Fprintf(os.Stderr, "manifest: staged %d font(s) in %s\n", n, dest)
		}
	}

	b, err := buildManifest(presets, *version, map[string]asset{"exe": exe, "setup": setup}, resolve, resolveF)
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

// joinRef builds an asset reference. With an empty base the reference stays relative, which is
// what removable media needs.
func joinRef(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

// stageFontFiles copies every font a preset references into dir, ready for upload or for a USB.
// Release assets are flat so they get a font- prefix; on removable media they keep their plain
// name inside fonts\.
func stageFontFiles(presets []presetSource, fontDir, dir string, local bool) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	n := 0
	for _, ps := range presets {
		if ps.FontFile == "" || seen[ps.FontFile] {
			continue
		}
		seen[ps.FontFile] = true
		b, err := os.ReadFile(filepath.Join(fontDir, ps.FontFile))
		if err != nil {
			fmt.Fprintf(os.Stderr, "manifest: warning: %v\n", err)
			continue
		}
		name := ps.FontFile
		if !local {
			name = "font-" + name
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
