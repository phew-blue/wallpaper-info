# wallpaper-info Installer + Preset Endpoint Implementation Plan

> ## ⚠️ Historical record — implemented, do not follow as instructions
>
> This plan was executed in August 2026. Two decisions changed during implementation, so
> parts of it describe a system that was never shipped:
>
> | Plan says | Actually shipped |
> |---|---|
> | Manifest hosted on the website (`Task 12` publish step, all of `Task 13`) | Manifest + backgrounds are **GitHub release assets**, read from `releases/latest/download/manifest.json`. No website hosting, no `WEBSITE_PUSH_TOKEN`, no HTTPRoute change |
> | `wallpaper-info.phew.blue` hostname | No subdomain. The site carries a listing entry on `phew.blue/software` only |
> | Purge history with `git-filter-repo` (`Task 2`) | No Python available, and branch rewrites would not have removed tags or `refs/pull/*`. History was squashed and the GitHub repo deleted/recreated — see the note on Task 2 |
>
> **`README.md` and `CLAUDE.md` are the current truth.** This file is kept to record what was
> planned and why it changed.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `wallpaper-info` as a per-user Windows installer with a system-tray app that pulls presets and updates from a public static manifest at `wallpaper-info.phew.blue`.

**Architecture:** Presets are TOML files committed in this repo; a generator turns them into `manifest.json`, served as static files from the existing `website` deployment. The Go binary gains a manifest/preset layer (cached, never blocking a render), a `RenderOpts` struct so layout is data-driven, a `getlantern/systray` tray mode, and a lexi-style silent self-update. The installer is Inno Setup compiled by `iscc` under Wine on the Linux `home-ops` runner.

**Tech Stack:** Go 1.23 (flat `package main`), `BurntSushi/toml`, `golang.org/x/image`, `getlantern/systray` v1.2.2, Inno Setup 6 under Wine, GitHub Actions on the self-hosted `home-ops` runner, Astro/nginx (`website` repo), Flux/HTTPRoute (`home-ops` repo).

**Spec:** `docs/superpowers/specs/2026-08-20-installer-and-preset-endpoint-design.md`

## Global Constraints

- Module is `github.com/phew-blue/wallpaper-info`, Go 1.23, **flat `package main`, no subdirectories** — except `tools/manifest`, which is its own `package main` binary.
- Platform-specific code always comes in pairs: `//go:build windows` and `//go:build !windows`. **Both sides of every pair must be kept in sync when a signature changes.**
- Windows release build must stay `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w"`. Any dependency that breaks this build is rejected.
- **Never block or fail a render on the network.** Manifest, background, and update failures degrade to cached data, then to existing behaviour. This mirrors the existing `publicIP()` fallback chain.
- Brand defaults are fixed: accent `#0092CA`, secondary `#6A7078`. Centered label defaults to hostname; `"-"` hides it.
- Config precedence, in order: **defaults < preset < config file < explicit flags.** Flags count as "explicit" only via `flag.Visit`, matching the existing merge in `main.go`.
- Manifest `schema` is an integer; an unrecognised value means **ignore the manifest entirely** rather than partially apply it.
- All downloaded artifacts (backgrounds, installers) carry a `sha256` and are verified before use.
- Tests are stdlib `testing` only — no test framework dependency. Run with `go test ./...`.
- Commits use Conventional Commits (`<type>(<scope>): <description>`). No `Co-Authored-By` trailers. Stage files explicitly by name; never `git add -A` or `git add .`.
- Windows-only paths (tray, update, wallpaper set) cannot be executed in CI or in the dev environment. Keep them thin; put all testable logic in platform-neutral files.

---

### Task 1: `--demo` mode and a sanitised preview image

The tracked `docs/preview.png` is a real render of a real machine: it shows a full name, hostname `LT-01`, LAN IPs, and a live WAN IP. The repo must go public, so the image must be regenerated from synthetic data — and that generation must be reproducible so future preset screenshots never leak again.

**Files:**
- Create: `demo.go`
- Create: `demo_test.go`
- Modify: `main.go` (add the `--demo` flag; use `DemoInfo()` in place of `Gather()`)
- Replace: `docs/preview.png` (regenerated output)

**Interfaces:**
- Consumes: `Info`, `NIC` from `info.go`; `Render` from `render.go` (still the current signature at this point — Task 3 changes it).
- Produces: `func DemoInfo() Info` — synthetic, deterministic system facts for screenshots and tests. Later tasks use `DemoInfo()` as fixture data instead of hand-building `Info` values.

- [ ] **Step 1: Write the failing test**

```go
// demo_test.go
package main

import "strings"

import "testing"

func TestDemoInfoIsSynthetic(t *testing.T) {
	got := DemoInfo()

	// Documentation-range addresses only (RFC 5737 / RFC 1918) — never a real WAN IP.
	if got.PublicIP != "203.0.113.42" {
		t.Errorf("PublicIP = %q, want the RFC 5737 documentation address", got.PublicIP)
	}
	if got.Host != "DEMO-PC" {
		t.Errorf("Host = %q, want DEMO-PC", got.Host)
	}
	if got.User != "demo" {
		t.Errorf("User = %q, want demo", got.User)
	}
	if len(got.Nics) == 0 {
		t.Fatal("Nics is empty; the preview should show at least one interface")
	}
	for _, n := range got.Nics {
		if !strings.HasPrefix(n.IP, "192.168.") {
			t.Errorf("NIC %s has IP %q, want a 192.168.0.0/16 private address", n.Name, n.IP)
		}
	}
}

func TestDemoInfoIsDeterministic(t *testing.T) {
	if DemoInfo().Sig() != DemoInfo().Sig() {
		t.Error("DemoInfo() is not deterministic; screenshots would not be reproducible")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestDemoInfo -v`
Expected: FAIL — `undefined: DemoInfo`

- [ ] **Step 3: Write minimal implementation**

```go
// demo.go
package main

// DemoInfo returns fixed, synthetic system facts for documentation screenshots and tests.
// Every address is from a documentation or private range so a published preview can never
// leak a real machine's identity or WAN address.
func DemoInfo() Info {
	return Info{
		User:     "demo",
		Host:     "DEMO-PC",
		OS:       "Windows 11 Pro (25H2)",
		Uptime:   "7h 8m",
		CPU:      "Intel Core i5-8265U · 8 cores",
		RAM:      "16 GiB RAM",
		Disk:     "C:  931 GiB · 86% free",
		PublicIP: "203.0.113.42", // RFC 5737 documentation range
		Nics: []NIC{
			{Name: "Ethernet", IP: "192.168.1.20"},
			{Name: "WiFi", IP: "192.168.1.21"},
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestDemoInfo -v`
Expected: PASS

- [ ] **Step 5: Wire the `--demo` flag into main.go**

Add alongside the other flag declarations:

```go
demo := flag.Bool("demo", false, "render synthetic sample data instead of this machine's facts (for docs)")
```

Then, inside the `do := func() error {` closure, replace `info := Gather()` with:

```go
		info := Gather()
		if *demo {
			info = DemoInfo()
		}
```

- [ ] **Step 6: Regenerate the preview image**

```bash
go build -o /tmp/wpi . && /tmp/wpi --demo --name DEMO-PC --out docs/preview.png
```

Open `docs/preview.png` and confirm by eye: no real name, hostname `DEMO-PC`, WAN `203.0.113.42`, LAN `192.168.1.x`. If any real value appears, stop — the flag is not being honoured.

- [ ] **Step 7: Run the full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add demo.go demo_test.go main.go docs/preview.png
git commit -m "feat(demo): --demo renders synthetic data; regenerate sanitised preview

The tracked preview was a real render exposing a WAN IP, full name, and LAN
topology. --demo makes doc screenshots reproducible and leak-free."
```

---

### Task 2: Purge the leaked preview from history and make the repo public

> **Done, but not as written below.** Two assumptions in this task were wrong:
> `git-filter-repo` is a Python program and this environment has no Python, and — more
> importantly — rewriting branches would not have removed the leak from GitHub at all. Tags
> `v0.1.0`/`v0.1.1` pointed into the leaked commits, a stale `docs/add-claude-md` branch still
> held the file, and `refs/pull/1/head` keeps PR heads fetchable forever on a public repo.
>
> What was actually done: the 12 pre-feature commits were squashed into one clean root commit
> with `git checkout --orphan`, the 13 feature commits were replayed onto it with
> `git rebase --onto`, and the GitHub repo was **deleted and recreated public**, which is the
> only step that reliably drops tags and PR refs. Verified from a fresh clone: all three blobs
> absent, WAN IP absent from every blob. Releases v0.1.0/v0.1.1 were lost with the repo; they
> are superseded by the installer flow. The steps below are kept for the record.

**This task rewrites published history and changes repo visibility. Both are irreversible in practice. Get explicit confirmation from Rob immediately before running the force-push and the visibility flip — do not batch this task with others.**

Three blobs must die: `185cddc3` (`f0a9e12`), `7447e6d4` (`5486523`), `b6f8f453` (`78fe820`).

**Files:**
- Modify: history only (no working-tree changes beyond what Task 1 committed)

**Interfaces:**
- Consumes: the sanitised `docs/preview.png` from Task 1.
- Produces: a public repo whose release assets are anonymously downloadable — every later task's manifest URLs depend on this.

- [ ] **Step 1: Confirm the leak is gone from the working tree, and only from there**

Run:
```bash
git log --all --oneline -- docs/preview.png
```
Expected: the three historical commits **plus** the Task 1 commit. This confirms the purge is still needed.

- [ ] **Step 2: Take a full backup before rewriting**

```bash
git clone --mirror . /tmp/wallpaper-info-backup.git
```
Expected: a complete mirror. Do not proceed without it.

- [ ] **Step 3: Purge the historical blobs**

`git-filter-repo` is the supported tool (`git filter-branch` is deprecated and slow). Install it if missing (`pipx install git-filter-repo` or `uv tool install git-filter-repo`).

```bash
git filter-repo --force --invert-paths --path docs/preview.png
```

This removes the file from **all** history, including the Task 1 commit. Re-add the sanitised image afterwards:

```bash
cp /tmp/wallpaper-info-backup.git/../preview-sanitised.png docs/preview.png 2>/dev/null || \
  (go build -o /tmp/wpi . && /tmp/wpi --demo --name DEMO-PC --out docs/preview.png)
git add docs/preview.png
git commit -m "docs: sanitised preview rendered with --demo"
```

- [ ] **Step 4: Verify no blob containing the leak survives**

```bash
git rev-list --objects --all | grep -i preview || echo "no preview blobs in history"
git log --all --oneline -- docs/preview.png
```
Expected: only the single new commit from Step 3. The three old SHAs must be unreachable:
```bash
git cat-file -e 185cddc30ea76b8970013f195a3242f128f58ee8 2>&1 || echo "purged"
```
Expected: `purged` (or an error) for each of `185cddc3`, `7447e6d4`, `b6f8f453`.

- [ ] **Step 5: Confirm with Rob, then force-push**

`git filter-repo` removes the `origin` remote by design. Re-add and push:

```bash
git remote add origin git@github.com:phew-blue/wallpaper-info.git
git push --force --all origin
git push --force --tags origin
```

- [ ] **Step 6: Purge GitHub's server-side cache of the old blobs**

Rewritten commits stay reachable on GitHub until garbage-collected. Ask GitHub Support to run `gc`, **or** simply accept that the repo is still private at this moment and the blobs become unreachable before the visibility flip. Verify from a fresh clone:

```bash
git clone git@github.com:phew-blue/wallpaper-info.git /tmp/verify-clone
cd /tmp/verify-clone && git log --all --oneline -- docs/preview.png
```
Expected: one commit only.

- [ ] **Step 7: Flip the repo to public**

```bash
gh repo edit phew-blue/wallpaper-info --visibility public --accept-visibility-change-consequences
gh repo view phew-blue/wallpaper-info --json visibility
```
Expected: `{"visibility":"PUBLIC"}`

- [ ] **Step 8: Verify anonymous release download works**

```bash
curl -sIL -o /dev/null -w '%{http_code}\n' \
  https://github.com/phew-blue/wallpaper-info/releases/latest
```
Expected: `200` without any credential. This is the assumption the whole manifest design rests on.

---

### Task 3: `RenderOpts` — data-driven rows and corner

`Render` currently takes six positional parameters and would reach nine. Replace the tail with a struct before adding anything else.

**Files:**
- Modify: `render.go:19` (signature and the line-building block)
- Modify: `main.go` (call site)
- Create: `render_test.go`

**Interfaces:**
- Consumes: `Info` from `info.go`.
- Produces:
  - `type Corner int` with constants `BottomRight`, `BottomLeft`, `TopRight`, `TopLeft`
  - `func ParseCorner(s string) (Corner, bool)` — accepts `"bottom-right"`, `"bottom-left"`, `"top-right"`, `"top-left"`
  - `type RenderOpts struct { Label, Font, Accent, Secondary string; Corner Corner; Rows []string }`
  - `func Render(bg image.Image, info Info, opts RenderOpts) image.Image`
  - `func infoLines(info Info, rows []string) []string` — the row selection, split out so it is testable without drawing
  - `DefaultRows` — `[]string{"user", "os", "uptime", "cpu", "ram", "disk", "nics", "wan"}`

- [ ] **Step 1: Write the failing test**

```go
// render_test.go
package main

import (
	"strings"
	"testing"
)

func TestInfoLinesDefaultsToAllRows(t *testing.T) {
	got := infoLines(DemoInfo(), nil)
	joined := strings.Join(got, "\n")

	for _, want := range []string{"demo", "DEMO-PC", "Windows 11 Pro", "uptime", "i5-8265U", "16 GiB", "931 GiB", "Ethernet", "203.0.113.42"} {
		if !strings.Contains(joined, want) {
			t.Errorf("default rows missing %q; got:\n%s", want, joined)
		}
	}
}

func TestInfoLinesSelectsAndOrdersRows(t *testing.T) {
	got := infoLines(DemoInfo(), []string{"wan", "os"})
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(got), got)
	}
	if !strings.Contains(got[0], "203.0.113.42") {
		t.Errorf("first line = %q, want the WAN row first", got[0])
	}
	if !strings.Contains(got[1], "Windows 11 Pro") {
		t.Errorf("second line = %q, want the OS row second", got[1])
	}
}

func TestInfoLinesNicsExpandsToOnePerInterface(t *testing.T) {
	got := infoLines(DemoInfo(), []string{"nics"})
	if len(got) != 2 {
		t.Fatalf("got %d lines, want one per NIC: %v", len(got), got)
	}
}

func TestInfoLinesIgnoresUnknownRow(t *testing.T) {
	got := infoLines(DemoInfo(), []string{"os", "does-not-exist"})
	if len(got) != 1 {
		t.Fatalf("got %d lines, want unknown rows skipped: %v", len(got), got)
	}
}

func TestParseCorner(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Corner
		ok   bool
	}{
		{"bottom-right", BottomRight, true},
		{"top-left", TopLeft, true},
		{"sideways", BottomRight, false},
		{"", BottomRight, false},
	} {
		got, ok := ParseCorner(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseCorner(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestInfoLines|TestParseCorner' -v`
Expected: FAIL — `undefined: infoLines`, `undefined: ParseCorner`

- [ ] **Step 3: Implement `Corner`, `RenderOpts`, and `infoLines`**

Add to `render.go`, above `Render`:

```go
// Corner selects which edge the info block is anchored to.
type Corner int

const (
	BottomRight Corner = iota // current, and the brand-wallpaper default
	BottomLeft
	TopRight
	TopLeft
)

// ParseCorner maps a config string onto a Corner. The bool reports whether the string was known;
// an unknown value must fall back to the default rather than silently moving the panel.
func ParseCorner(s string) (Corner, bool) {
	switch s {
	case "bottom-right":
		return BottomRight, true
	case "bottom-left":
		return BottomLeft, true
	case "top-right":
		return TopRight, true
	case "top-left":
		return TopLeft, true
	}
	return BottomRight, false
}

func (c Corner) right() bool { return c == BottomRight || c == TopRight }
func (c Corner) top() bool   { return c == TopLeft || c == TopRight }

// DefaultRows is the full row set, in the order the panel has always rendered them.
var DefaultRows = []string{"user", "os", "uptime", "cpu", "ram", "disk", "nics", "wan"}

// RenderOpts carries everything about how the panel looks, so Render keeps a two-argument
// data/one-argument-options shape as more presentation knobs arrive.
type RenderOpts struct {
	Label     string // centered label ("" = none)
	Font      string // family name or .ttf path
	Accent    string // hex
	Secondary string // hex
	Corner    Corner
	Rows      []string // nil or empty = DefaultRows
}

// infoLines turns the requested rows into display strings. Unknown row names are skipped so a
// newer manifest naming a row this build does not know cannot blank the panel.
func infoLines(info Info, rows []string) []string {
	if len(rows) == 0 {
		rows = DefaultRows
	}
	var out []string
	for _, r := range rows {
		switch r {
		case "user":
			out = append(out, info.User+"  @  "+info.Host)
		case "os":
			out = append(out, info.OS)
		case "uptime":
			out = append(out, "uptime "+info.Uptime)
		case "cpu":
			out = append(out, info.CPU)
		case "ram":
			out = append(out, info.RAM)
		case "disk":
			out = append(out, info.Disk)
		case "nics":
			for _, n := range info.Nics {
				out = append(out, n.Name+":  "+n.IP)
			}
		case "wan":
			out = append(out, "WAN:  "+info.PublicIP)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestInfoLines|TestParseCorner' -v`
Expected: PASS

- [ ] **Step 5: Change `Render` to take `RenderOpts`**

Replace the signature at `render.go:19` and rebuild the `lines` slice from `infoLines`. The gap rule stays as-is: a gap before the `os` group, before the `cpu` group, and before the first network line. Preserve the accent+semibold styling on the first row only.

```go
func Render(bg image.Image, info Info, opts RenderOpts) image.Image {
	b := bg.Bounds()
	W, H := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(dst, dst.Bounds(), bg, b.Min, draw.Src)

	accent := parseHex(opts.Accent, color.RGBA{0x00, 0x92, 0xCA, 0xff})
	sec := parseHex(opts.Secondary, color.RGBA{0x6A, 0x70, 0x78, 0xff})

	secSize := float64(W) * 0.0072
	reg := face(secSize, false, opts.Font)
	semi := face(secSize, true, opts.Font)

	if opts.Label != "" {
		nameSize := float64(W) * 0.016
		nameFace := face(nameSize, true, opts.Font)
		tracking := fixed.Int26_6(nameSize * 0.42 * 64)
		drawTracked(dst, nameFace, accent, strings.ToUpper(opts.Label), W/2, int(float64(H)*0.745), tracking)
	}

	texts := infoLines(info, opts.Rows)
	// ... build the []line from texts: index 0 gets (accent, semi); the rest (sec, reg).
	// Gaps: apply g before the row that starts the os, cpu, and network groups, as today.
```

Anchoring: keep the existing `marginR`/`marginB` arithmetic for `BottomRight`. For `opts.Corner.right() == false`, mirror the x anchor to `marginR` from the left and left-align instead of right-align. For `opts.Corner.top()`, place the first baseline at `marginB` from the top and advance downwards instead of computing back from the bottom edge.

- [ ] **Step 6: Update the call site in main.go**

```go
		corner, _ := ParseCorner(cfg.Layout.Corner) // Task 4 adds cfg.Layout; until then pass BottomRight
		img := Render(bg, info, RenderOpts{
			Label:     label,
			Font:      cfg.Font,
			Accent:    cfg.Accent,
			Secondary: cfg.Secondary,
			Corner:    corner,
			Rows:      cfg.Layout.Rows,
		})
```

- [ ] **Step 7: Verify the default render is byte-identical to before**

Regressions here are invisible in tests but obvious on a desktop. Compare against the pre-change binary:

```bash
git stash && go build -o /tmp/wpi-before . && git stash pop
go build -o /tmp/wpi-after .
/tmp/wpi-before --demo --name DEMO-PC --out /tmp/before.png
/tmp/wpi-after  --demo --name DEMO-PC --out /tmp/after.png
cmp /tmp/before.png /tmp/after.png && echo "IDENTICAL"
```
Expected: `IDENTICAL`. If it differs, the refactor changed default output — fix before committing.

- [ ] **Step 8: Run the full suite and commit**

```bash
go test ./... && go vet ./...
git add render.go render_test.go main.go
git commit -m "refactor(render): RenderOpts struct with data-driven rows and corner"
```

---

### Task 4: Config gains `preset` and `[layout]`, with documented precedence

**Files:**
- Modify: `config.go` (add `Preset`, `Layout`)
- Modify: `main.go` (apply precedence)
- Modify: `config.example.toml`
- Create: `config_test.go`

**Interfaces:**
- Consumes: `Config`, `LoadConfig`, `DefaultConfig` from `config.go`.
- Produces:
  - `type Layout struct { Corner string; Rows []string }`
  - `Config.Preset string`, `Config.Layout Layout`
  - `func MergePreset(cfg Config, p Preset, explicit map[string]bool) Config` — applies preset values only where the user has not explicitly set them. `Preset` itself is defined in Task 5; until then, this task's tests use a local stub. **To avoid a forward dependency, implement `MergePreset` in Task 6 and limit this task to the config fields.**

- [ ] **Step 1: Write the failing test**

```go
// config_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsPresetAndLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
preset = "phew-blue"
accent = "#112233"

[layout]
corner = "top-left"
rows = ["os", "wan"]
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Preset != "phew-blue" {
		t.Errorf("Preset = %q, want phew-blue", cfg.Preset)
	}
	if cfg.Accent != "#112233" {
		t.Errorf("Accent = %q, want the file value to beat the default", cfg.Accent)
	}
	if cfg.Layout.Corner != "top-left" {
		t.Errorf("Layout.Corner = %q, want top-left", cfg.Layout.Corner)
	}
	if len(cfg.Layout.Rows) != 2 || cfg.Layout.Rows[0] != "os" || cfg.Layout.Rows[1] != "wan" {
		t.Errorf("Layout.Rows = %v, want [os wan]", cfg.Layout.Rows)
	}
}

func TestLoadConfigMissingFileKeepsBrandDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Accent != "#0092CA" || cfg.Secondary != "#6A7078" {
		t.Errorf("defaults = %q/%q, want #0092CA/#6A7078", cfg.Accent, cfg.Secondary)
	}
	if cfg.Preset != "" || cfg.Layout.Corner != "" || cfg.Layout.Rows != nil {
		t.Error("absent config should leave preset/layout zero so a preset can fill them")
	}
}

func TestSaveConfigRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := DefaultConfig()
	want.Preset = "phew-blue"
	want.Layout = Layout{Corner: "bottom-left", Rows: []string{"user", "wan"}}

	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset != want.Preset || got.Layout.Corner != want.Layout.Corner || len(got.Layout.Rows) != 2 {
		t.Errorf("round-trip lost data: got %+v, want %+v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestLoadConfig|TestSaveConfig' -v`
Expected: FAIL — `cfg.Preset undefined`, `undefined: Layout`

- [ ] **Step 3: Add the fields**

```go
// Layout controls which rows appear and where the panel sits. Zero values mean "unset", so a
// preset can supply them without overriding an explicit local choice.
type Layout struct {
	Corner string   `toml:"corner"` // bottom-right (default) | bottom-left | top-right | top-left
	Rows   []string `toml:"rows"`   // ordered subset of user, os, uptime, cpu, ram, disk, nics, wan
}

type Config struct {
	Base      string `toml:"base"`
	Name      string `toml:"name"`
	Font      string `toml:"font"`
	Accent    string `toml:"accent"`
	Secondary string `toml:"secondary"`
	Watch     int    `toml:"watch"`
	Preset    string `toml:"preset"` // id of the last applied preset ("" = purely local config)
	Layout    Layout `toml:"layout"`
}
```

`DefaultConfig` is unchanged — `Preset` and `Layout` must stay zero so Task 6 can tell "unset" from "explicitly chosen".

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestLoadConfig|TestSaveConfig' -v`
Expected: PASS

- [ ] **Step 5: Document the fields in config.example.toml**

```toml
# Preset id last applied from https://wallpaper-info.phew.blue/manifest.json.
# Values from the preset are used only where you have not set them yourself:
# defaults < preset < this file < command-line flags.
preset = "phew-blue"

[layout]
# bottom-right (default) | bottom-left | top-right | top-left
corner = "bottom-right"
# Ordered subset of: user, os, uptime, cpu, ram, disk, nics, wan
rows = ["user", "os", "uptime", "cpu", "ram", "disk", "nics", "wan"]
```

- [ ] **Step 6: Run the full suite and commit**

```bash
go test ./... && go vet ./...
git add config.go config_test.go config.example.toml
git commit -m "feat(config): preset id and [layout] rows/corner settings"
```

---

### Task 5: `manifest.go` — fetch, cache, and schema-guard

**Files:**
- Create: `manifest.go`
- Create: `manifest_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `const ManifestSchema = 1`
  - `const DefaultManifestURL = "https://wallpaper-info.phew.blue/manifest.json"`
  - `type Asset struct { URL, SHA256 string; Size int64 }`
  - `type Background struct { W, H int; URL, SHA256 string }`
  - `type Preset struct { ID, Name, Description, Accent, Secondary, Font, Label string; Layout Layout; Backgrounds []Background }`
  - `type Latest struct { Version string; Exe, Setup Asset }`
  - `type Manifest struct { Schema int; Latest Latest; Presets []Preset }`
  - `func ParseManifest(b []byte) (Manifest, error)`
  - `func (m Manifest) Preset(id string) (Preset, bool)`
  - `type ManifestFetcher struct { URL, CachePath string; TTL time.Duration; Client *http.Client; Now func() time.Time }`
  - `func (f ManifestFetcher) Get() (Manifest, error)` — fresh cache → network → stale cache → error
  - `func ManifestCachePath() string` — `%LOCALAPPDATA%\wallpaper-info\manifest.json`, else `~/.cache/wallpaper-info/manifest.json`

- [ ] **Step 1: Write the failing test**

```go
// manifest_test.go
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
	dir := t.TempDir()
	cache := filepath.Join(dir, "manifest.json")
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
	dir := t.TempDir()
	cache := filepath.Join(dir, "manifest.json")
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
	dir := t.TempDir()
	cache := filepath.Join(dir, "manifest.json")

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
		URL:       srv.URL,
		CachePath: filepath.Join(t.TempDir(), "absent.json"),
		TTL:       time.Hour, Client: srv.Client(), Now: time.Now,
	}
	if _, err := f.Get(); err == nil {
		t.Error("want an error when there is neither cache nor network")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestParseManifest|TestFetcher' -v`
Expected: FAIL — `undefined: ParseManifest`

- [ ] **Step 3: Implement manifest.go**

```go
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

const DefaultManifestURL = "https://wallpaper-info.phew.blue/manifest.json"

type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

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

type Latest struct {
	Version string `json:"version"`
	Exe     Asset  `json:"exe"`
	Setup   Asset  `json:"setup"`
}

type Manifest struct {
	Schema  int      `json:"schema"`
	Latest  Latest   `json:"latest"`
	Presets []Preset `json:"presets"`
}

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
// stale cache. Callers must treat any error as "carry on with local config".
type ManifestFetcher struct {
	URL       string
	CachePath string
	TTL       time.Duration
	Client    *http.Client
	Now       func() time.Time
}

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

func (f ManifestFetcher) Get() (Manifest, error) {
	if b, mod, err := f.readCache(); err == nil && f.Now().Sub(mod) < f.TTL {
		if m, err := ParseManifest(b); err == nil {
			return m, nil
		}
	}

	if b, err := f.fetch(); err == nil {
		if m, err := ParseManifest(b); err == nil {
			f.writeCache(b) // best-effort; a read-only cache dir must not fail the fetch
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestParseManifest|TestFetcher' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
go test ./... && go vet ./...
git add manifest.go manifest_test.go
git commit -m "feat(manifest): fetch/parse presets + latest version with cached fallback"
```

---

### Task 6: `preset.go` — apply a preset and cache its background

**Files:**
- Create: `preset.go`
- Create: `preset_test.go`

**Interfaces:**
- Consumes: `Preset`, `Background` (Task 5); `Config`, `Layout` (Task 4).
- Produces:
  - `func PickBackground(bgs []Background, screenW int) (Background, bool)` — nearest by width
  - `func ApplyPreset(cfg Config, p Preset, explicit map[string]bool) Config`
  - `func BackgroundCachePath(sha string) string`
  - `func EnsureBackground(bg Background, client *http.Client) (string, error)` — returns a local path, downloading and sha256-verifying on a cache miss
  - `func VerifySHA256(path, want string) error`

- [ ] **Step 1: Write the failing test**

```go
// preset_test.go
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
	for _, tc := range []struct{ screen int; want string }{
		{3840, "4k"}, {1920, "hd"}, {2560, "qhd"},
		{2000, "hd"},   // 80 away from hd, 560 from qhd
		{3000, "qhd"},  // 440 from qhd, 840 from 4k
		{800, "hd"},    // below everything: nearest is still hd
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
		{"hostname", ""},      // "" means "use the hostname", the existing convention
		{"none", "-"},         // "-" hides the label
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
	t.Setenv("HOME", t.TempDir())

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

	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("HOME", dir)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestPickBackground|TestApplyPreset|TestEnsureBackground' -v`
Expected: FAIL — `undefined: PickBackground`

- [ ] **Step 3: Implement preset.go**

```go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
// defaults < preset < config file < explicit flags; explicit holds the flag names from flag.Visit.
func ApplyPreset(cfg Config, p Preset, explicit map[string]bool) Config {
	set := func(name string) bool { return !explicit[name] }

	if set("accent") && p.Accent != "" {
		cfg.Accent = p.Accent
	}
	if set("secondary") && p.Secondary != "" {
		cfg.Secondary = p.Secondary
	}
	if set("font") && p.Font != "" {
		cfg.Font = p.Font
	}
	if set("name") {
		switch p.Label {
		case "hostname":
			cfg.Name = "" // "" already means "use the hostname"
		case "none":
			cfg.Name = "-"
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

// BackgroundCachePath is content-addressed, so a changed preset naturally re-downloads and old
// files can be deleted without bookkeeping.
func BackgroundCachePath(sha string) string {
	base := ""
	if runtime.GOOS == "windows" {
		base = os.Getenv("LOCALAPPDATA")
	}
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "wallpaper-info", "backgrounds", sha+".png")
}

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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestPickBackground|TestApplyPreset|TestEnsureBackground' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
go test ./... && go vet ./...
git add preset.go preset_test.go
git commit -m "feat(preset): apply presets and cache verified backgrounds"
```

---

### Task 7: Wire presets into main, add `--preset` and `--list-presets`

**Files:**
- Modify: `main.go`
- Modify: `info.go` (extend `Sig()`)
- Create: `sig_test.go`

**Interfaces:**
- Consumes: everything from Tasks 3–6.
- Produces: `func (i Info) SigWith(cfg Config) string` — a fingerprint covering both the facts and the presentation, so `--watch` re-renders after a preset change.

- [ ] **Step 1: Write the failing test**

```go
// sig_test.go
package main

import "testing"

func TestSigWithChangesWhenPresetChanges(t *testing.T) {
	info := DemoInfo()
	a := DefaultConfig()
	a.Preset = "phew-blue"
	b := a
	b.Preset = "mono"

	if info.SigWith(a) == info.SigWith(b) {
		t.Error("changing preset must change the signature or --watch will skip the re-render")
	}
}

func TestSigWithChangesWhenLayoutChanges(t *testing.T) {
	info := DemoInfo()
	a := DefaultConfig()
	a.Layout = Layout{Corner: "bottom-right", Rows: []string{"os"}}
	b := a
	b.Layout = Layout{Corner: "top-left", Rows: []string{"os"}}
	c := a
	c.Layout = Layout{Corner: "bottom-right", Rows: []string{"os", "wan"}}

	if info.SigWith(a) == info.SigWith(b) {
		t.Error("corner change not reflected in the signature")
	}
	if info.SigWith(a) == info.SigWith(c) {
		t.Error("row change not reflected in the signature")
	}
}

func TestSigWithStableForSameInputs(t *testing.T) {
	info := DemoInfo()
	cfg := DefaultConfig()
	if info.SigWith(cfg) != info.SigWith(cfg) {
		t.Error("signature is not stable")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestSigWith -v`
Expected: FAIL — `info.SigWith undefined`

- [ ] **Step 3: Implement `SigWith`**

Add to `info.go`, leaving the existing `Sig()` in place as the facts-only fingerprint:

```go
// SigWith fingerprints the facts *and* the presentation, so a preset or layout change forces a
// re-render even when nothing about the machine has changed.
func (i Info) SigWith(cfg Config) string {
	s := i.Sig() + "|preset=" + cfg.Preset +
		"|corner=" + cfg.Layout.Corner +
		"|accent=" + cfg.Accent + "|secondary=" + cfg.Secondary +
		"|font=" + cfg.Font + "|name=" + cfg.Name + "|base=" + cfg.Base
	for _, r := range cfg.Layout.Rows {
		s += "|row=" + r
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestSigWith -v`
Expected: PASS

- [ ] **Step 5: Add the flags and the preset-resolution step to main.go**

Declare alongside the existing flags:

```go
	preset := flag.String("preset", "", "apply a named preset from the manifest")
	listPresets := flag.Bool("list-presets", false, "list presets available in the manifest and exit")
	manifestURL := flag.String("manifest", "", "manifest URL (default "+DefaultManifestURL+")")
```

Record explicit flags while merging (extending the existing `flag.Visit` block):

```go
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		explicit[f.Name] = true
		switch f.Name {
		// ... existing cases unchanged ...
		case "preset":
			cfg.Preset = *preset
		}
	})
```

Then, after the config merge and before the render loop:

```go
	// Presets are best-effort: a machine with no network must still render from local config.
	if cfg.Preset != "" || *listPresets {
		fetcher := NewManifestFetcher(*manifestURL)
		m, err := fetcher.Get()
		switch {
		case err != nil && *listPresets:
			fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
			os.Exit(1)
		case err != nil:
			fmt.Fprintln(os.Stderr, "wallpaper-info: preset unavailable, using local config:", err)
		default:
			if *listPresets {
				for _, p := range m.Presets {
					fmt.Printf("%-16s %s\n", p.ID, p.Description)
				}
				return
			}
			p, ok := m.Preset(cfg.Preset)
			if !ok {
				fmt.Fprintf(os.Stderr, "wallpaper-info: preset %q not in manifest, using local config\n", cfg.Preset)
				break
			}
			cfg = ApplyPreset(cfg, p, explicit)
			if !explicit["base"] && cfg.Base == "" {
				if bg, ok := PickBackground(p.Backgrounds, ScreenWidth()); ok {
					if path, err := EnsureBackground(bg, &http.Client{Timeout: 60 * time.Second}); err == nil {
						cfg.Base = path
					} else {
						fmt.Fprintln(os.Stderr, "wallpaper-info: background:", err)
					}
				}
			}
		}
	}
```

Change the skip check inside `do` from `sig == lastSig` to use `info.SigWith(cfg)`.

- [ ] **Step 6: Add `ScreenWidth()` as a platform pair**

`info_windows.go` — via `GetSystemMetrics(SM_CXSCREEN)` from `user32.dll`:

```go
var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

// ScreenWidth is the primary display's width in pixels, used to pick the nearest background.
func ScreenWidth() int {
	r, _, _ := procGetSystemMetrics.Call(0) // SM_CXSCREEN
	if r == 0 {
		return 1920
	}
	return int(r)
}
```

`info_other.go`:

```go
// ScreenWidth: no display query off Windows; 1920 keeps background selection deterministic.
func ScreenWidth() int { return 1920 }
```

- [ ] **Step 7: Verify end to end against a local manifest**

```bash
go build -o /tmp/wpi .
python3 -m http.server 8099 --directory /tmp/manifest-fixture &
/tmp/wpi --manifest http://localhost:8099/manifest.json --list-presets
```
Expected: the preset ids print. Then confirm the offline path degrades rather than fails:
```bash
/tmp/wpi --manifest http://localhost:1/manifest.json --preset phew-blue --demo --out /tmp/x.png
echo "exit=$?"
```
Expected: a warning on stderr, `exit=0`, and `/tmp/x.png` written. **A network failure must never fail a render.**

- [ ] **Step 8: Run the full suite and commit**

```bash
go test ./... && go vet ./...
git add main.go info.go info_windows.go info_other.go sig_test.go
git commit -m "feat: --preset/--list-presets with offline-safe manifest resolution"
```

---

### Task 8: Preset sources and the manifest generator

**Files:**
- Create: `presets/phew-blue.toml`
- Create: `presets/mono.toml`
- Create: `tools/manifest/main.go`
- Create: `tools/manifest/main_test.go`

**Interfaces:**
- Consumes: the JSON shape from Task 5 (the generator must emit exactly what `ParseManifest` accepts).
- Produces: `manifest.json` on stdout or at `-out`.

- [ ] **Step 1: Write the preset sources**

`presets/phew-blue.toml`:

```toml
id          = "phew-blue"
name        = "phew-blue (default)"
description = "Brand wallpaper, cyan accent, hostname label"
accent      = "#0092CA"
secondary   = "#6A7078"
font        = "Open Sans"
label       = "hostname"

[layout]
corner = "bottom-right"
rows   = ["user", "os", "uptime", "cpu", "ram", "disk", "nics", "wan"]

# Widths must match the directories under brand/assets/wallpapers/logo/png.
backgrounds = ["3840x2160", "2560x1440", "1920x1080"]
```

`presets/mono.toml`:

```toml
id          = "mono"
name        = "Mono"
description = "Greyscale, no WAN IP, top-left panel"
accent      = "#C8CDD2"
secondary   = "#6A7078"
font        = "Open Sans"
label       = "hostname"

[layout]
corner = "top-left"
rows   = ["user", "os", "uptime", "cpu", "ram", "disk", "nics"]

backgrounds = ["3840x2160", "2560x1440", "1920x1080"]
```

- [ ] **Step 2: Write the failing test**

```go
// tools/manifest/main_test.go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./tools/manifest/ -v`
Expected: FAIL — `undefined: presetSource`

- [ ] **Step 4: Implement the generator**

`tools/manifest/main.go` reads `presets/*.toml` into `presetSource`, resolves each `WxH` string into a `Background` (URL + sha256 computed from the actual PNG under the backgrounds directory), and marshals with `json.MarshalIndent`. Key parts:

```go
type layoutSource struct {
	Corner string   `toml:"corner"`
	Rows   []string `toml:"rows"`
}

type presetSource struct {
	ID          string       `toml:"id"`
	Name        string       `toml:"name"`
	Description string       `toml:"description"`
	Accent      string       `toml:"accent"`
	Secondary   string       `toml:"secondary"`
	Font        string       `toml:"font"`
	Label       string       `toml:"label"`
	Layout      layoutSource `toml:"layout"`
	Backgrounds []string     `toml:"backgrounds"` // "WxH"
}

type asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// resolveBG returns the public URL and sha256 for one preset background.
type resolveBG func(presetID, size string) (url, sha string)

func buildManifest(presets []presetSource, version string, assets map[string]asset, resolve resolveBG) ([]byte, error) {
	// Parse each "WxH" with fmt.Sscanf; return a descriptive error if it does not match.
	// Emit {"schema":1,"latest":{...},"presets":[...]} via json.MarshalIndent(v, "", "  ").
}
```

Flags: `-presets ./presets`, `-backgrounds <dir>`, `-version <tag>`, `-exe/-setup <file>` (for size+sha256), `-base-url https://wallpaper-info.phew.blue`, `-out manifest.json`.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./tools/manifest/ -v`
Expected: PASS

- [ ] **Step 6: Verify the generator's output round-trips through the client parser**

This is the contract between the two halves of the system; assert it rather than eyeballing it.

```bash
go run ./tools/manifest -version v0.0.0-test -out /tmp/manifest.json
go test ./... -run TestParseManifest
```
Then add a test in `manifest_test.go` that parses `/tmp/manifest.json` if present, or simply confirm manually:
```bash
go run ./tools/manifest -version v0.0.0-test | head -30
```
Expected: valid JSON with `"schema": 1` and both presets.

- [ ] **Step 7: Commit**

```bash
go test ./... && go vet ./...
git add presets/phew-blue.toml presets/mono.toml tools/manifest/main.go tools/manifest/main_test.go
git commit -m "feat(manifest): preset sources and the manifest generator"
```

---

### Task 9: Tray mode

Windows-only and not executable here, so keep the logic thin and delegate to functions already covered by tests.

**Files:**
- Create: `tray_windows.go`
- Create: `tray_other.go`
- Create: `assets/wallpaper-info.ico`
- Modify: `main.go` (`--tray`)
- Modify: `go.mod` / `go.sum`

**Interfaces:**
- Consumes: `Config`, `ApplyPreset`, `ManifestFetcher`, the render closure from `main.go`.
- Produces: `func RunTray(app *App) error` — blocks until Quit; returns an error if the tray cannot start so the caller can fall back to headless watch.
- Requires `main.go` to expose an `App` struct holding the config, the render function, and the manifest fetcher, rather than the current closure-over-locals.

- [ ] **Step 1: Add the dependency and confirm the release build still works**

The build constraint is non-negotiable: a dependency that breaks `CGO_ENABLED=0 -H windowsgui` is rejected.

```bash
go get github.com/getlantern/systray@v1.2.2
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o /tmp/wpi.exe .
```
Expected: builds clean. (Verified during design: 4.2 MB, no CGO.)

- [ ] **Step 2: Extract an `App` struct in main.go**

```go
// App is the run-time state the tray and the watch loop both drive.
type App struct {
	Cfg      Config
	CfgPath  string
	Demo     bool
	Out      string
	Explicit map[string]bool
	Fetcher  ManifestFetcher
	lastSig  string
}

// RenderOnce renders and sets the wallpaper. force skips the unchanged-signature check.
func (a *App) RenderOnce(force bool) error { /* the current do() body, using a.Cfg */ }

// ApplyPresetByID switches preset at runtime and re-renders.
func (a *App) ApplyPresetByID(id string) error {
	m, err := a.Fetcher.Get()
	if err != nil {
		return err
	}
	p, ok := m.Preset(id)
	if !ok {
		return fmt.Errorf("preset %q not found", id)
	}
	a.Cfg = ApplyPreset(a.Cfg, p, a.Explicit)
	if bg, ok := PickBackground(p.Backgrounds, ScreenWidth()); ok {
		if path, err := EnsureBackground(bg, &http.Client{Timeout: 60 * time.Second}); err == nil {
			a.Cfg.Base = path
		}
	}
	if err := SaveConfig(a.CfgPath, a.Cfg); err != nil {
		return err
	}
	return a.RenderOnce(true)
}
```

- [ ] **Step 3: Write tray_other.go (the stub half of the pair)**

```go
//go:build !windows

package main

import "errors"

// RunTray: the tray is Windows-only. Callers fall back to the headless watch loop.
func RunTray(app *App) error { return errors.New("tray mode is only supported on Windows") }
```

- [ ] **Step 4: Write tray_windows.go**

```go
//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/getlantern/systray"
)

//go:embed assets/wallpaper-info.ico
var trayIcon []byte

func RunTray(app *App) error {
	systray.Run(func() { onTrayReady(app) }, func() {})
	return nil
}

func onTrayReady(app *App) {
	systray.SetIcon(trayIcon)
	systray.SetTitle("wallpaper-info")
	systray.SetTooltip("wallpaper-info — " + version)

	mRefresh := systray.AddMenuItem("Refresh now", "Re-render the wallpaper")
	mPresets := systray.AddMenuItem("Presets", "Switch preset")
	mUpdate := systray.AddMenuItem("Check for updates", "")
	mConfig := systray.AddMenuItem("Open config", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "")

	// Presets are a submenu built from the manifest; a fetch failure leaves a disabled entry
	// rather than an empty menu, so the cause is visible on the desktop.
	if m, err := app.Fetcher.Get(); err == nil {
		for _, p := range m.Presets {
			p := p
			item := mPresets.AddSubMenuItemCheckbox(p.Name, p.Description, p.ID == app.Cfg.Preset)
			go func() {
				for range item.ClickedCh {
					if err := app.ApplyPresetByID(p.ID); err != nil {
						fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
					}
				}
			}()
		}
	} else {
		mPresets.AddSubMenuItem("(manifest unavailable)", err.Error()).Disable()
	}

	go func() {
		for {
			select {
			case <-mRefresh.ClickedCh:
				if err := app.RenderOnce(true); err != nil {
					fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
				}
			case <-mUpdate.ClickedCh:
				if err := CheckAndUpdate(app, true); err != nil {
					fmt.Fprintln(os.Stderr, "wallpaper-info: update:", err)
				}
			case <-mConfig.ClickedCh:
				_ = exec.Command("explorer.exe", app.CfgPath).Start()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	// The tray owns the refresh loop in --tray mode, replacing the --watch sleep loop.
	go func() {
		for app.Cfg.Watch > 0 {
			time.Sleep(time.Duration(app.Cfg.Watch) * time.Minute)
			if err := app.RenderOnce(false); err != nil {
				fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
			}
		}
	}()
}
```

- [ ] **Step 5: Create the icon**

Export a 256×256 .ico from the brand mark:

```bash
cp ../brand/assets/logo/phew-blue-mark.png /tmp/mark.png
magick /tmp/mark.png -resize 256x256 -define icon:auto-resize=256,64,48,32,16 assets/wallpaper-info.ico
```
If ImageMagick is unavailable, ask Rob for a .ico from the brand repo. **`go:embed` fails the build if the file is missing**, so this must exist before the next step.

- [ ] **Step 6: Wire `--tray` into main.go**

```go
	if *tray {
		if err := RunTray(app); err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info: tray unavailable, running headless:", err)
			// fall through to the watch loop — a shell problem must not cost the wallpaper
		} else {
			return
		}
	}
```

- [ ] **Step 7: Verify both platforms build**

```bash
go build ./... && go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o /tmp/wpi.exe .
go test ./...
```
Expected: all pass. Tray behaviour itself must be confirmed manually on a Windows machine — note that in the commit message.

- [ ] **Step 8: Commit**

```bash
git add tray_windows.go tray_other.go assets/wallpaper-info.ico main.go go.mod go.sum
git commit -m "feat(tray): --tray system tray with preset switching

Menu behaviour needs manual verification on Windows; cross-compile only in CI."
```

---

### Task 10: Silent self-update

**Files:**
- Create: `update_windows.go`
- Create: `update_other.go`
- Create: `update_test.go`
- Modify: `main.go` (`--update`, `version` var)

**Interfaces:**
- Consumes: `Manifest`, `Asset`, `VerifySHA256`.
- Produces:
  - `var version = "dev"` (set via `-ldflags "-X main.version=..."`)
  - `func NeedsUpdate(current, latest string) bool` — platform-neutral, tested
  - `func CheckAndUpdate(app *App, userInitiated bool) error`

- [ ] **Step 1: Write the failing test**

```go
// update_test.go
package main

import "testing"

func TestNeedsUpdate(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"v0.3.0", "v0.4.0", true},
		{"v0.4.0", "v0.4.0", false},
		{"v0.4.1", "v0.4.0", false}, // never downgrade
		{"0.3.0", "v0.4.0", true},   // tolerate a missing v prefix
		{"dev", "v0.4.0", false},    // a dev build must never self-update
		{"v0.3.0", "", false},       // no version advertised: do nothing
		{"v0.9.0", "v0.10.0", true}, // numeric compare, not lexical
	} {
		if got := NeedsUpdate(tc.current, tc.latest); got != tc.want {
			t.Errorf("NeedsUpdate(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestNeedsUpdate -v`
Expected: FAIL — `undefined: NeedsUpdate`

- [ ] **Step 3: Implement `NeedsUpdate` in a platform-neutral file**

Put it in `update.go` (no build tag) so it is testable on Linux; keep only the Windows-specific execution in `update_windows.go`.

```go
package main

import (
	"strconv"
	"strings"
)

// NeedsUpdate reports whether latest is strictly newer than current. "dev" builds never update:
// a developer's local binary must not be replaced by a release.
func NeedsUpdate(current, latest string) bool {
	if current == "dev" || current == "" || latest == "" {
		return false
	}
	return compareSemver(strings.TrimPrefix(latest, "v"), strings.TrimPrefix(current, "v")) > 0
}

func compareSemver(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		an, bn := 0, 0
		if i < len(as) {
			an, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bn, _ = strconv.Atoi(bs[i])
		}
		switch {
		case an > bn:
			return 1
		case an < bn:
			return -1
		}
	}
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestNeedsUpdate -v`
Expected: PASS

- [ ] **Step 5: Implement the Windows update path**

`update_windows.go` — lexi's proven sequence: download, verify, run silently, exit and let the installer restart the app.

```go
//go:build windows

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func CheckAndUpdate(app *App, userInitiated bool) error {
	m, err := app.Fetcher.Get()
	if err != nil {
		return err
	}
	if !NeedsUpdate(version, m.Latest.Version) {
		if userInitiated {
			fmt.Println("wallpaper-info: already up to date (" + version + ")")
		}
		return nil
	}

	tmp := filepath.Join(os.TempDir(), "wallpaper-info-setup.exe")
	if err := download(m.Latest.Setup, tmp); err != nil {
		return err
	}
	// The installer replaces this running exe, so hand off and exit rather than waiting.
	cmd := exec.Command(tmp, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART")
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

func download(a Asset, dst string) error {
	resp, err := http.Get(a.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	// Never execute an unverified installer.
	if err := VerifySHA256(dst, a.SHA256); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}
```

`update_other.go`:

```go
//go:build !windows

package main

import "errors"

// CheckAndUpdate: self-update is Windows-only; elsewhere the binary is built from source.
func CheckAndUpdate(app *App, userInitiated bool) error {
	return errors.New("self-update is only supported on Windows")
}
```

- [ ] **Step 6: Add `--update` and the version variable to main.go**

```go
// version is stamped at build time: -ldflags "-X main.version=$TAG". "dev" never self-updates.
var version = "dev"
```

```go
	if *update {
		if err := CheckAndUpdate(app, true); err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info: update:", err)
			os.Exit(1)
		}
		return
	}
```

Also add `--version` printing `version` — the installer and support both need it.

- [ ] **Step 7: Verify both builds and the version stamp**

```bash
go test ./... && go vet ./...
go build -ldflags "-X main.version=v9.9.9" -o /tmp/wpi . && /tmp/wpi --version
```
Expected: `v9.9.9`

- [ ] **Step 8: Commit**

```bash
git add update.go update_windows.go update_other.go update_test.go main.go
git commit -m "feat(update): sha256-verified silent self-update via the manifest"
```

---

### Task 11: Inno Setup installer

**Files:**
- Create: `installer/wallpaper-info.iss`
- Create: `.github/actions/wine-inno/action.yml` (vendored from lexi)

**Interfaces:**
- Consumes: `wallpaper-info-windows-amd64.exe` from the Go build; `MyAppVersion` passed by CI.
- Produces: `installer/Output/wallpaper-info-setup-<version>.exe`, which Task 12 uploads and the manifest advertises as `latest.setup`.

- [ ] **Step 1: Vendor lexi's Wine + Inno action**

A cross-repo composite action would require checking out `lexi` on every run; copying keeps this repo self-contained.

```bash
mkdir -p .github/actions/wine-inno
cp ../lexi/.github/actions/wine-inno/action.yml .github/actions/wine-inno/action.yml
```
Read the copied file and confirm it has no lexi-specific paths. It outputs `iscc` (the path to ISCC.exe) and caches the Wine prefix under key `wine-inno-6.7.1-v1`.

- [ ] **Step 2: Write the installer script**

`installer/wallpaper-info.iss`:

```inno
; wallpaper-info — per-user Windows installer (Inno Setup 6)
;
; Build:  iscc /DMyAppVersion=0.4.0 installer\wallpaper-info.iss
; Output: installer\Output\wallpaper-info-setup-{version}.exe
;
; Pre-requisite: wallpaper-info.exe next to this script (CI copies it in).

#define MyAppName      "wallpaper-info"
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#define MyAppPublisher "Phew Blue"
#define MyAppURL       "https://wallpaper-info.phew.blue"
#define MyAppExeName   "wallpaper-info.exe"

[Setup]
AppId={{7C1B6A54-2E4D-4C2F-9E1A-0B7D3F5A9C21}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
; Per-user: nothing here needs elevation, and installing under LOCALAPPDATA lets the running
; exe be replaced during self-update (the same reason lexi installs there).
PrivilegesRequired=lowest
DefaultDirName={localappdata}\wallpaper-info
DisableDirPage=yes
DefaultGroupName={#MyAppName}
OutputDir=Output
OutputBaseFilename=wallpaper-info-setup-{#MyAppVersion}
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#MyAppExeName}

[Files]
Source: "wallpaper-info.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--tray"
; Startup shortcut: the tray process refreshes the wallpaper on a timer.
Name: "{userstartup}\phew-blue wallpaper-info"; Filename: "{app}\{#MyAppExeName}"; Parameters: "--tray"; Tasks: startup

[Tasks]
Name: "startup"; Description: "Run wallpaper-info at logon (tray)"; GroupDescription: "Startup"

[Run]
; Apply the chosen preset and paint the wallpaper immediately, then leave the tray running.
Filename: "{app}\{#MyAppExeName}"; Parameters: "--preset {code:GetPreset}"; Flags: runhidden waituntilterminated
Filename: "{app}\{#MyAppExeName}"; Parameters: "--tray"; Flags: runhidden nowait postinstall; Description: "Start wallpaper-info now"

[UninstallDelete]
Type: filesandordirs; Name: "{localappdata}\wallpaper-info"

[Code]
var PresetPage: TInputOptionWizardPage;

// /PRESET=<id> supports unattended installs from provisioning; the wizard page is the
// interactive equivalent. Keep the ids in sync with presets/*.toml.
function GetPreset(Param: string): string;
var i: Integer;
begin
  Result := ExpandConstant('{param:PRESET|phew-blue}');
  if (PresetPage <> nil) and (ExpandConstant('{param:PRESET|}') = '') then
  begin
    for i := 0 to PresetPage.CheckListBox.Items.Count - 1 do
      if PresetPage.SelectedValueIndex = i then
        Result := PresetPage.CheckListBox.Items[i];
  end;
end;

procedure InitializeWizard;
begin
  PresetPage := CreateInputOptionPage(wpSelectTasks,
    'Choose a look', 'Which preset should the wallpaper use?',
    'You can change this later from the tray icon.', True, False);
  PresetPage.Add('phew-blue');
  PresetPage.Add('mono');
  PresetPage.SelectedValueIndex := 0;
end;
```

- [ ] **Step 3: Build the installer locally under Wine to verify the script compiles**

If Wine + Inno are available locally:
```bash
cp /tmp/wpi.exe installer/wallpaper-info.exe
wine "$HOME/.wine/drive_c/Program Files (x86)/Inno Setup 6/ISCC.exe" \
  /DMyAppVersion=0.0.0 installer/wallpaper-info.iss
ls -la installer/Output/
```
Expected: `wallpaper-info-setup-0.0.0.exe`. If Wine is not installed locally, this is verified by the CI run in Task 12 instead — say so explicitly rather than claiming it was tested.

- [ ] **Step 4: Add the build artifact to .gitignore**

```bash
printf 'installer/Output/\ninstaller/wallpaper-info.exe\n' >> .gitignore
```

- [ ] **Step 5: Commit**

```bash
git add installer/wallpaper-info.iss .github/actions/wine-inno/action.yml .gitignore
git commit -m "feat(installer): per-user Inno Setup installer with preset selection"
```

---

### Task 12: Release workflow builds the installer and publishes the manifest

**Files:**
- Modify: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: Tasks 8 and 11.
- Produces: a GitHub release with `wallpaper-info-windows-amd64.exe` + `wallpaper-info-setup-<v>.exe`, and a `manifest.json` committed to the `website` repo.

- [ ] **Step 1: Stamp the version into the Go build**

In the existing build step, change the build line to:

```bash
VERSION="${GITHUB_REF_NAME}"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=${VERSION}" \
  -o wallpaper-info-windows-amd64.exe .
```

- [ ] **Step 2: Add the installer build steps**

```yaml
      - name: Setup Wine + Inno Setup
        id: inno
        uses: ./.github/actions/wine-inno

      - name: Build installer
        run: |
          set -euo pipefail
          VERSION="${GITHUB_REF_NAME#v}"
          cp wallpaper-info-windows-amd64.exe installer/wallpaper-info.exe
          xvfb-run -a wine "${{ steps.inno.outputs.iscc }}" \
            "/DMyAppVersion=${VERSION}" installer/wallpaper-info.iss
          ls -la installer/Output/
```

- [ ] **Step 3: Upload both artifacts, then generate the manifest**

The manifest must be generated **after** the release assets exist, so it can never advertise a version whose assets failed to upload.

```yaml
      - uses: softprops/action-gh-release@b4309332981a82ec1c5618f44dd2e27cc8bfbfda # v3
        with:
          files: |
            wallpaper-info-windows-amd64.exe
            installer/Output/wallpaper-info-setup-*.exe
          generate_release_notes: true

      - name: Verify assets are anonymously downloadable
        run: |
          set -euo pipefail
          for f in wallpaper-info-windows-amd64.exe "wallpaper-info-setup-${GITHUB_REF_NAME#v}.exe"; do
            url="https://github.com/${GITHUB_REPOSITORY}/releases/download/${GITHUB_REF_NAME}/${f}"
            code=$(curl -sIL -o /dev/null -w '%{http_code}' "$url")
            [ "$code" = "200" ] || { echo "asset not public: $url ($code)"; exit 1; }
          done

      - name: Generate manifest
        run: |
          set -euo pipefail
          go run ./tools/manifest \
            -version "${GITHUB_REF_NAME}" \
            -exe wallpaper-info-windows-amd64.exe \
            -setup "installer/Output/wallpaper-info-setup-${GITHUB_REF_NAME#v}.exe" \
            -backgrounds ./backgrounds \
            -out manifest.json
          cat manifest.json
```

- [ ] **Step 4: Publish the manifest to the website repo** — *replaced: the manifest is
  uploaded as a release asset in the same step as the binaries; there is no cross-repo
  push and no `WEBSITE_PUSH_TOKEN`.*

```yaml
      - name: Publish manifest to website
        env:
          GH_TOKEN: ${{ secrets.WEBSITE_PUSH_TOKEN }}
        run: |
          set -euo pipefail
          git clone --depth 1 "https://x-access-token:${GH_TOKEN}@github.com/phew-blue/website.git" /tmp/website
          mkdir -p /tmp/website/public/wallpaper-info
          cp manifest.json /tmp/website/public/wallpaper-info/manifest.json
          cd /tmp/website
          git config user.name  'Phew-Blue-Bot'
          git config user.email 'bot@phew.blue'
          git add public/wallpaper-info/manifest.json
          git commit -m "chore(wallpaper-info): manifest for ${GITHUB_REF_NAME}" || exit 0
          git push
```

`WEBSITE_PUSH_TOKEN` must be a repo secret with `contents:write` on `phew-blue/website`. Create it with `gh secret set WEBSITE_PUSH_TOKEN -R phew-blue/wallpaper-info` before the first tagged release, or this step fails the release.

- [ ] **Step 5: Validate the workflow file**

```bash
gh workflow view release --repo phew-blue/wallpaper-info 2>/dev/null || \
  python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('YAML OK')"
```
Expected: `YAML OK`

- [ ] **Step 6: Commit, then verify with a real pre-release tag**

```bash
git add .github/workflows/release.yml
git commit -m "ci: build the installer under Wine and publish the manifest"
git push
git tag v0.4.0-rc1 && git push origin v0.4.0-rc1
gh run watch
```
Expected: green run; both assets on the release; `manifest.json` committed to `website`. **Do not report this task complete on a green YAML parse alone — the Wine/Inno step is the risky part and only a real run proves it.**

---

### Task 13: Serve the manifest at wallpaper-info.phew.blue

> **Not shipped.** Superseded entirely: the manifest and backgrounds became GitHub
> release assets, so nothing is hosted on the website and no cluster change was made.
> The website got a one-file listing entry (`src/content/software/wallpaper-info.md`)
> on its existing `/software` page instead. The steps below were not carried out.

**Files:**
- Modify (website repo): `package.json` (background sync script)
- Create (website repo): `public/wallpaper-info/.gitkeep`
- Modify (home-ops repo): `kubernetes/apps/default/website/app/helmrelease.yaml`

**Interfaces:**
- Consumes: `manifest.json` pushed by Task 12.
- Produces: `https://wallpaper-info.phew.blue/manifest.json` and `.../backgrounds/<preset>/<WxH>.png`.

- [ ] **Step 1: Add the second hostname to the existing HTTPRoute**

In `kubernetes/apps/default/website/app/helmrelease.yaml`, under `route.app.hostnames`:

```yaml
    route:
      app:
        hostnames:
          - "phew.blue"
          - "wallpaper-info.phew.blue"
```

Confirm the gateway's TLS covers the new name before relying on it:

```bash
kubectl -n kube-system get gateway external -o yaml | grep -A15 'listeners:'
kubectl -n default get httproute website -o yaml | grep -A5 hostnames
```
If the cert is a wildcard for `*.phew.blue`, nothing more is needed; if it is per-host, the certificate must list the new name.

- [ ] **Step 2: Sync preset backgrounds from the brand repo at build time**

Add to the website repo's `package.json` scripts, following the existing `sync:brand` precedent:

```json
    "sync:wallpaper-info": "node scripts/sync-wallpaper-backgrounds.mjs"
```

The script copies `../brand/assets/wallpapers/logo/png/<WxH>/phew-blue-wallpaper.png` to `public/wallpaper-info/backgrounds/phew-blue/<WxH>.png` for each size named in the presets, and the greyscale variant for `mono`. Wire it into `build`.

- [ ] **Step 3: Verify locally before deploying**

```bash
cd ../website && npm run sync:wallpaper-info && npm run build
ls -la dist/wallpaper-info/
```
Expected: `manifest.json` and the `backgrounds/` tree present in `dist/`.

- [ ] **Step 4: Commit both repos**

```bash
cd ../website
git add package.json scripts/sync-wallpaper-backgrounds.mjs public/wallpaper-info/.gitkeep
git commit -m "feat(wallpaper-info): serve preset manifest and backgrounds"

cd ../home-ops
git add kubernetes/apps/default/website/app/helmrelease.yaml
git commit -m "feat(default/website): add wallpaper-info.phew.blue hostname"
```

- [ ] **Step 5: Verify the live endpoint after Flux reconciles**

```bash
curl -s https://wallpaper-info.phew.blue/manifest.json | head -20
curl -sI https://wallpaper-info.phew.blue/manifest.json | head -3
```
Expected: HTTP 200 and valid JSON with `"schema": 1`. Then prove the client half works against production:
```bash
go run . --list-presets
```
Expected: the preset ids from the live manifest.

---

### Task 14: Simplify the provisioning script

**Files:**
- Modify (windows-setup repo): `wallpaper-info.ps1`

**Interfaces:**
- Consumes: the public installer URL from the manifest.
- Produces: a much shorter script — the brand-checkout dependency, resolution picker, `gh` auth, and manual Startup shortcut all move into the installer and manifest.

- [ ] **Step 1: Replace the download + shortcut logic**

```powershell
# Fetch the installer advertised by the public manifest — no gh auth, no brand checkout.
$manifest = Invoke-RestMethod https://wallpaper-info.phew.blue/manifest.json
$setup    = Join-Path $env:TEMP 'wallpaper-info-setup.exe'
Invoke-WebRequest -Uri $manifest.latest.setup.url -OutFile $setup

# Verify before executing.
$hash = (Get-FileHash $setup -Algorithm SHA256).Hash.ToLower()
if ($hash -ne $manifest.latest.setup.sha256) { throw "wallpaper-info: installer sha256 mismatch" }

$preset = if ($MachineName) { 'phew-blue' } else { 'phew-blue' }
Start-Process -FilePath $setup -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',"/PRESET=$preset" -Wait
```

- [ ] **Step 2: Keep the lock/logon screen block unchanged**

That block needs elevation and is out of the per-user installer's scope. Leave it, but source its image from the installed background cache rather than the brand checkout.

- [ ] **Step 3: Verify on a real machine**

This script only runs meaningfully on Windows. Run it on a test machine and confirm: ARP entry present, wallpaper painted, tray icon visible after logon, `--watch` shortcut gone (replaced by the installer's `--tray` entry).

- [ ] **Step 4: Commit**

```bash
git add wallpaper-info.ps1
git commit -m "refactor(wallpaper-info): install via the public installer + manifest"
```

---

### Task 15: Documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update README**

Cover: install from `https://wallpaper-info.phew.blue` (the installer link), the tray menu, `--preset` / `--list-presets` / `--update` / `--demo`, and how to add a preset (`presets/<id>.toml` → tagged release regenerates the manifest).

- [ ] **Step 2: Update CLAUDE.md**

The existing file describes each source file; add the new ones and correct the statements this work invalidates:

- "No tests exist" → `go test ./...` covers manifest, preset, config, render rows, and version compare
- Add `manifest.go`, `preset.go`, `demo.go`, `tray_*.go`, `update*.go`, `tools/manifest`, `presets/`, `installer/`
- Record the config precedence rule (defaults < preset < file < flags)
- Record that the repo is public and why (installer needs anonymous release downloads)
- Update Consumers: `windows-setup` now runs the installer rather than downloading the exe with `gh`
- Note the gotcha: **`docs/preview.png` must only ever be regenerated with `--demo`**

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: installer, tray, presets, and the manifest endpoint"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Repo becomes public | 2 |
| Installer: Inno Setup | 11 |
| Tray: getlantern/systray | 9 |
| Endpoint: website sub-path + second hostname | 13 |
| Manifest schema + rules | 5 |
| Preset authoring + generator | 8 |
| Client preset layer (manifest.go, preset.go) | 5, 6, 7 |
| Update layer | 10 |
| Config + precedence | 4, 6 |
| RenderOpts / layout | 3 |
| `Info.Sig()` includes preset | 7 |
| Release flow | 12 |
| Consumer change (windows-setup) | 14 |
| Error handling table | 5, 6, 7, 9, 10 (each degrades as specified) |
| Testing section | 1, 3, 4, 5, 6, 7, 8, 10 |
| Risk: leaked preview | 1, 2 (found during history audit; upgraded from a spec note to two tasks) |

**Gaps found and closed:** the spec assumed the history was clean; the audit found the WAN-IP leak, so Tasks 1 and 2 were added ahead of everything else. The spec's `MergePreset` name was inconsistent with the implemented `ApplyPreset`; the plan uses `ApplyPreset` throughout and Task 4's interface block notes the deferral.

**Type consistency:** `Preset`, `Background`, `Asset`, `Layout`, `RenderOpts`, `Corner`, and `App` are defined once (Tasks 3, 4, 5, 9) and referenced with those exact names afterwards. `ApplyPreset(cfg, p, explicit)` keeps the same signature in Tasks 6, 7, and 9.
