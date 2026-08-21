# wallpaper-info: installer, tray mode, and preset endpoint

Date: 2026-08-20
Status: approved design, ready for implementation planning

## Problem

`wallpaper-info` ships as a bare `.exe` with no install story. Provisioning
(`windows-setup/wallpaper-info.ps1`) downloads it with `gh release download`, which
requires an authenticated `gh` against a **private** repo, then hand-rolls a Startup
shortcut. Consequences:

- No install/uninstall entry: removing it means deleting files and a shortcut by hand.
- No update path: a new release only lands on a machine that re-runs provisioning.
- No way to change the look on a running machine short of editing TOML over RDP.
- Backgrounds come from a local checkout of the private `brand` repo, so a machine
  without that checkout cannot get a branded base image.

## Goals

1. A real Windows installer: per-user, no admin, with an Add/Remove Programs entry.
2. A resident tray app so a user can refresh, switch preset, and update from the desktop.
3. A public HTTP endpoint serving a **preset catalogue** plus **latest-version** info, so
   both the installer and the tray app can pull presets and updates without credentials.

## Non-goals

- No preset *authoring* UI. Presets are committed TOML in this repo.
- No telemetry, check-in, or central management plane (lexi's hub model is out of scope —
  there is no server-side state here; the manifest is a static file).
- No live usage metrics on the wallpaper. It still shows specs, not utilisation.
- No MSI / Intune / GPO deployment path (see Decisions).

## Decisions

These were settled during brainstorming; the reasoning matters more than the choice.

### Repo becomes public

`phew-blue/wallpaper-info` is currently private, which is the root cause of the `gh auth`
dependency. It contains a wallpaper renderer and no secrets. **Making it public** lets the
manifest point at GitHub release asset URLs that any machine can fetch anonymously, and
keeps binaries out of the website's container image.

### Installer: Inno Setup, not WiX/MSI

`lexi` already compiles Inno Setup installers on the **Linux** `home-ops` runner by running
`iscc` under Wine, via a cached composite action (`lexi/.github/actions/wine-inno`). Reusing
that toolchain means no new dotnet/WiX build path and one installer technology across
phew-blue. Cost accepted: no MSI, so no Intune/GPO deployment story. Revisit only if these
machines come under MDM.

### Tray: `github.com/getlantern/systray` v1.2.2

The same library `lexi`'s relay and agent use, proven on these Windows machines. Verified to
cross-compile `windows/amd64` with `CGO_ENABLED=0 -ldflags "-H windowsgui"` (4.2 MB binary).
`energye/systray` is the maintained fork and produces a smaller binary (1.9 MB), but
consistency with a library already running in the fleet outweighs that. Hand-rolled
`Shell_NotifyIconW` syscalls were rejected: Windows GUI code cannot be executed in the
development environment, so an unverifiable message loop is the riskier option.

### Endpoint: sub-path of the existing website

> **Superseded during implementation.** Once the repo was public, hosting the generated
> catalogue on the website was an extra hop for no benefit: the presets already live in git.
> `manifest.json` and the background PNGs became **GitHub release assets**, read from
> `releases/latest/download/manifest.json`. That removed the cross-repo push, the
> `WEBSITE_PUSH_TOKEN` secret, the copied images, the sync script, and the HTTPRoute change.
> The website keeps a one-file listing entry on `phew.blue/software`. The original reasoning
> is kept below.

`website` (Astro → nginx, `ghcr.io/phew-blue/website`) already serves the `phew.blue` apex
via an HTTPRoute in `home-ops`. Presets are small static files, so they ship in that image
rather than as a new deployment. `wallpaper-info.phew.blue` is added as a **second hostname
on the existing HTTPRoute**, so both `wallpaper-info.phew.blue/manifest.json` and
`phew.blue/wallpaper-info/manifest.json` resolve to the same files. Binaries stay on GitHub
releases (possible because the repo goes public), so a wallpaper-info release does not force
a website image rebuild.

## Architecture

```
wallpaper-info repo                     website repo                  client (Windows)
───────────────────                     ────────────                  ────────────────
presets/*.toml      ──generate──▶  public/wallpaper-info/       ──▶  manifest.json (24h cache)
                                     manifest.json                     │
brand/ wallpapers   ──build copy─▶   backgrounds/<id>/<WxH>.png  ──▶  background (sha256 cache)
                                                                       │
tag v* ──▶ GitHub release: .exe + setup.exe ─────────────────────▶  --update / installer
```

Three units, each independently understandable:

- **manifest service** — static JSON + PNGs over HTTPS. No server logic.
- **client preset layer** — fetch, cache, and apply a preset to the existing `Config`.
- **installer + tray** — packaging and the resident UI.

## Component 1: the manifest

Served at `https://wallpaper-info.phew.blue/manifest.json`. Schema:

```json
{
  "schema": 1,
  "latest": {
    "version": "v0.4.0",
    "exe":   { "url": "https://github.com/phew-blue/wallpaper-info/releases/download/v0.4.0/wallpaper-info-windows-amd64.exe", "sha256": "…", "size": 4227584 },
    "setup": { "url": "https://github.com/phew-blue/wallpaper-info/releases/download/v0.4.0/wallpaper-info-setup-0.4.0.exe", "sha256": "…", "size": 5100000 }
  },
  "presets": [
    {
      "id": "phew-blue",
      "name": "phew-blue (default)",
      "description": "Brand wallpaper, cyan accent, hostname label",
      "accent": "#0092CA",
      "secondary": "#6A7078",
      "font": "Open Sans",
      "label": "hostname",
      "layout": {
        "corner": "bottom-right",
        "rows": ["user", "os", "uptime", "cpu", "ram", "disk", "nics", "wan"]
      },
      "backgrounds": [
        { "w": 3840, "h": 2160, "url": "https://wallpaper-info.phew.blue/backgrounds/phew-blue/3840x2160.png", "sha256": "…" },
        { "w": 1920, "h": 1080, "url": "https://wallpaper-info.phew.blue/backgrounds/phew-blue/1920x1080.png", "sha256": "…" }
      ]
    }
  ]
}
```

Rules:

- `schema` is an integer. A client that sees a `schema` it does not know **ignores the
  manifest entirely** and carries on with local config — an unknown future format must never
  break a running desktop.
- `label` is `"hostname"`, `"none"`, or a literal string, mapping onto the existing
  `Config.Name` convention (`""`, `"-"`, literal).
- `backgrounds` is a list; the client picks the entry nearest the primary screen width, the
  same nearest-resolution rule `wallpaper-info.ps1` uses today.
- All downloads carry a `sha256`; the client verifies before use and discards on mismatch.

### Authoring and generation

Presets are committed as `presets/<id>.toml` in this repo — the same TOML vocabulary as
`config.toml` plus `id`/`name`/`description`/`backgrounds`. A generator
(`tools/manifest`, `go run ./tools/manifest`) reads `presets/*.toml`, resolves release asset
URLs and sha256s, and emits `manifest.json`. Background PNGs are copied from `../brand` at
website build time, following the precedent of the website's existing `sync:brand` script.

## Component 2: client preset + update layer

New files, matching the flat `package main` layout:

- **`manifest.go`** — fetch and parse the manifest; cache the raw JSON at
  `%LOCALAPPDATA%\wallpaper-info\manifest.json` with a 24h TTL. On any failure (offline,
  DNS, 5xx, bad JSON, unknown schema) fall back to the cached copy, then to nothing. This
  mirrors the existing `publicIP()` discipline: **never block a render on the network.**
- **`preset.go`** — apply a preset to a `Config`; download and cache backgrounds at
  `%LOCALAPPDATA%\wallpaper-info\backgrounds\<sha256>.png` (content-addressed, so a changed
  preset naturally re-downloads and old files are collectable).
- **`update_windows.go` / `update_other.go`** — compare `manifest.latest.version` against the
  build version, download `setup.exe`, verify sha256, run
  `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`, then `os.Exit(0)` and let the installer restart
  the app. This is lexi's proven update sequence. Non-Windows is a stub.

Version is stamped at build time via `-ldflags "-X main.version=…"`; it defaults to `dev`,
and a `dev` build never self-updates.

### Config and flag changes

`Config` gains:

```toml
preset = "phew-blue"     # preset id last applied ("" = none, purely local config)

[layout]
corner = "bottom-right"
rows   = ["user", "os", "uptime", "cpu", "ram", "disk", "nics", "wan"]
```

Precedence, extending the existing rule (flags override config only when explicitly set via
`flag.Visit`): **defaults < preset < config file < explicit flags.** A preset supplies values;
anything the user has explicitly set locally wins over it. This keeps a preset from silently
stomping a hand-tuned machine.

New flags: `--preset <id>`, `--list-presets`, `--manifest <url>` (default baked in),
`--tray`, `--update`.

### Render layout options

`Render`'s signature already carries six positional parameters and would reach nine. Replace
the tail with an options struct:

```go
type RenderOpts struct {
    Label, Font, Accent, Secondary string
    Corner Corner       // bottom-right (current behaviour) | bottom-left | top-right | top-left
    Rows   []string     // ordered subset of user, os, uptime, cpu, ram, disk, nics, wan
}

func Render(bg image.Image, info Info, opts RenderOpts) image.Image
```

The current line-building block becomes a table driven by `Rows`; an empty `Rows` means the
present-day full list, so existing configs render identically. `Corner` changes only the
margin/anchor arithmetic — the vw-relative sizing that matches the brand CSS is untouched.

`Info.Sig()` must fold in the preset id and layout, or `--watch`/`--tray` will skip the
re-render after a preset switch.

## Component 3: tray

`tray_windows.go` (build tag `windows`) and `tray_other.go` (a stub returning "unsupported"),
per the existing platform-pair convention. An `.ico` is embedded with `go:embed`, as lexi does.

Menu:

- **Refresh now** — force a render, bypassing the `Sig()` skip
- **Presets ▸** — radio list from the manifest; selecting one applies it and re-renders
- **Check for updates** — the `--update` path, with a balloon on "already current"
- **Open config** — `explorer.exe` on the config file
- **Run at logon** — checkbox toggling the Startup shortcut
- **Quit**

`--tray` subsumes `--watch`: it runs the same interval loop with the icon present. `--watch`
stays for headless use. The installer's shortcut uses `--tray`.

## Component 4: installer

`installer/wallpaper-info.iss`, modelled on `lexi/installer/relay/relay.iss`:

- `PrivilegesRequired=lowest` — unlike lexi, nothing here needs elevation.
- `DefaultDirName={localappdata}\wallpaper-info` — so a running exe can be replaced during
  self-update, the same reason lexi installs there.
- Startup shortcut → `wallpaper-info.exe --tray`.
- `MyAppVersion` passed in from CI; `AppId` fixed so upgrades replace in place.
- Optional preset page: a wizard dropdown listing presets from the manifest, defaulting to
  `phew-blue`; `/PRESET=<id>` supports unattended installs.
- Uninstall removes the shortcut, `%LOCALAPPDATA%` data, and optionally the config.

## Component 5: release flow

`.github/workflows/release.yml` gains steps after the existing Go build:

1. Build `wallpaper-info-windows-amd64.exe` (unchanged, plus `-X main.version=$TAG`).
2. Set up Wine + Inno via a copy of lexi's `wine-inno` composite action, vendored into
   `.github/actions/wine-inno` (a cross-repo composite action would require checking out
   `lexi`; copying keeps this repo self-contained).
3. `iscc` → `wallpaper-info-setup-<version>.exe`.
4. Attach both artifacts to the GitHub release.
5. `go run ./tools/manifest` → `manifest.json` with real URLs and sha256s.
6. Commit `manifest.json` (and any changed backgrounds) to the `website` repo, which triggers
   that repo's own image build and Flux rollout.

## Consumer change

`windows-setup/wallpaper-info.ps1` collapses to: download `setup.exe` from the manifest, run
`/VERYSILENT /PRESET=phew-blue`. The brand-checkout dependency, the nearest-resolution picker,
the manual Startup shortcut, and the `gh` auth requirement all move into the installer and the
manifest. The lock/logon-screen registry work stays in the PowerShell script — it needs
elevation and is out of the installer's per-user scope.

## Error handling

| Failure | Behaviour |
|---|---|
| Manifest unreachable | Use cached manifest; if none, run from local config. Never fail a render. |
| Unknown `schema` | Ignore the manifest wholesale; log once. |
| Background download fails or sha256 mismatch | Discard; fall back to cached background → current wallpaper → solid canvas (the existing `LoadBase` chain). |
| Update download fails | Balloon the error, keep running the old version. |
| Preset id not in manifest | Keep current settings; surface in `--list-presets` output and the tray. |
| Tray fails to initialise | Fall back to headless watch rather than exiting — a machine must not lose its wallpaper because the shell is unhappy. |

## Testing

The repo currently has no tests; this adds the first ones, for the logic that is genuinely
platform-independent and worth pinning:

- Manifest parse: valid, unknown-schema, malformed, missing fields.
- Nearest-resolution background selection.
- Config precedence: defaults < preset < file < explicit flags.
- `RenderOpts.Rows` selection and ordering, including the empty-means-all default.
- `Info.Sig()` changes when preset or layout changes.
- Cache TTL and fallback-to-stale behaviour, with an injected clock and HTTP client.

Windows-only paths (tray, update, wallpaper set) stay manually verified on a real machine —
they cannot be executed in CI or in the development environment, and the design deliberately
keeps unverifiable code thin by leaning on a proven tray library and lexi's update sequence.

## Risks

- **Repo going public** is one-way in practice. Confirm no history contains anything sensitive
  before flipping it (the repo is small and asset-free, so this is expected to be clean).
- **Wine + Inno on the runner** is proven for lexi but adds ~a few minutes and an apt install
  to a release that currently takes under a minute. The cached Wine prefix mitigates this.
- **getlantern/systray is unmaintained upstream.** Isolating all tray calls in
  `tray_windows.go` keeps a swap to `energye/systray` or raw syscalls to one file.
- **Two-repo release** (binaries here, manifest in `website`) means a partial failure can leave
  the manifest advertising a version whose assets are missing. Generate the manifest only
  after the release assets are confirmed uploaded.
