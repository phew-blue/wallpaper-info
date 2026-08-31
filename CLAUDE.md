# wallpaper-info

A single Go binary (flat `package main`, no subdirectories) that composites a system-info panel — `user @ host`, OS, uptime, CPU, RAM, disk, per-NIC LAN IPs, WAN IP — plus an optional centered label onto a background image and sets it as the Windows desktop wallpaper. Self-contained: the font is loaded from the system (Open Sans, else Segoe UI) and the background defaults to the current wallpaper, so it ships with no assets. Shows **specs, not live usage**.

---

## How It Works

- `main.go` — flag parsing, config merge (flags override config only when explicitly set via `flag.Visit`), the render-or-skip loop
- `config.go` — TOML config (`BurntSushi/toml`). On Windows: `%APPDATA%\Phew Blue\wallpaper-info\config.toml` for a fresh install, or the pre-existing `%APPDATA%\wallpaper-info\config.toml` for an upgraded one. Elsewhere `~/.config/wallpaper-info/config.toml`; `--write-config` persists the current effective settings. `manifest` names the **preset** catalogue (URL or local path) for a machine that does not use the published one — update checks ignore it
- `info.go` — cross-platform facts (user, host, NICs, public IP via `api.ipify.org`, cached 15 min); `Info.Sig()` fingerprints everything shown so `--watch` skips re-rendering when nothing changed
- `info_windows.go` / `info_other.go` — platform facts (registry + kernel32 syscalls on Windows; non-Windows returns stubs)
- `base.go` — background selection: explicit `--base` path → current wallpaper → solid brand-dark canvas
- `render.go` — drawing via `golang.org/x/image` (opentype); sizes are vw-relative to match the brand wallpaper's CSS (`.machine` = 1.6vw, tag strip = 0.72vw)
- `wallpaper_windows.go` / `wallpaper_other.go` — `SystemParametersInfoW` set/get; non-Windows can only `--out` a PNG
- `app.go` — `App` holds run-time state (config, manifest fetcher) so the tray, the watch loop, and one-shot runs all drive the same `RenderOnce`/`ApplyPresetByID`
- `manifest.go` — fetches/parses the catalogue from http(s) **or a local path** (USB / air-gapped): fresh cache → network → stale cache → give up. Relative asset URLs resolve against the manifest's own location so removable media is drive-letter independent. An unknown `schema` is ignored wholesale
- `preset.go` — applies a preset to `Config`, picks the nearest-resolution background, caches it content-addressed by sha256
- `demo.go` — `DemoInfo()`, the synthetic facts behind `--demo`
- `tray_windows.go` / `tray_other.go` — `getlantern/systray` tray icon (embedded `.ico`); non-Windows returns an error so the caller falls back to headless watch. Two icons are embedded and one is chosen at startup from `SystemUsesLightTheme`: white glyph for a dark taskbar, brand blue for a light one
- `assets/generate.py` — regenerates the tray icons from the two vendored SVGs (the mark, and the Material Symbols `image` glyph). Artwork is drawn at 16/24/32/48/256 rather than one large image, so the tray does not have to downsample an outline into a smudge
- `update.go` + `update_windows.go` / `update_other.go` — `NeedsUpdate` version compare (platform-neutral, tested) and the silent installer hand-off. `CheckAndUpdate` forces a network manifest read (a copy of the fetcher with `TTL` 0) so a scheduled check is never answered from a day-old cache; the tray runs it 5 min after start and daily thereafter
- `console_windows.go` / `console_other.go` — frees a console Windows opened just for us, so the resident daemon shows no console window while CLI use still prints
- `presets/*.toml` — authored preset sources (`background_set` lets a colour variant reuse another preset's images, `font_file` ships a font with the preset); `tools/manifest` turns them into the published `manifest.json` and stages the background and font assets
- `backgrounds/<set>/<WxH>.png` — brand wallpapers vendored so a release is self-contained
- `fonts/*.ttf` — fonts a preset can ship, so the panel renders identically on a machine that has never had Open Sans installed
- `installer/wallpaper-info.iss` — per-user Inno Setup installer, compiled by `iscc` under Wine on the Linux runner. `/PRESET=<id>` and `/MANIFEST=<url-or-path>` drive an unattended install; the latter points the post-install render at a USB or air-gapped catalogue

Windows is the real target; non-Windows builds exist so development works anywhere.

## Build / Run

```bash
go build -o wallpaper-info.exe .          # mise pins the Go toolchain (.mise.toml)

./wallpaper-info.exe                      # composite onto current wallpaper, set it
./wallpaper-info.exe --out preview.png    # render a PNG instead (only option on non-Windows)
./wallpaper-info.exe --watch 30           # re-render every 30 min
```

`go test ./...` covers the platform-independent logic: manifest parse/cache, preset application and precedence, background selection, config round-trip, render row selection, and version compare. Windows-only paths (tray, update, wallpaper set) cannot run in CI or in a Linux dev environment and are verified manually. A release builds the **same source twice**, differing only in subsystem — the `python.exe`/`pythonw.exe` pattern:

- `wallpaper-info.exe` — console subsystem. The one you type at a prompt, and the one `install.ps1` calls. A shell reads the PE subsystem field to decide whether to wait for the process and wire up its stdout, so this **must** stay console or `--version`/`--list-presets` silently return nothing (measured: 0 captures in 10 runs of a GUI-subsystem build).
- `wallpaper-infow.exe` — `-H windowsgui`. Everything launched unattended points here: both `[Icons]` shortcuts and the `[Run] --tray` entry. Windows creates and shows a console for a console-subsystem exe launched from a shortcut *before any of our code runs*, so a shortcut can only be silent if its target never asks for a console. `FreeConsole` afterwards just hides a window the user has already seen, with whatever was printed still in it — which is how "preset not in manifest" ended up on screen at logon.

`DetachConsole()` still exists and still runs for the resident modes, covering someone launching `wallpaper-info.exe --tray` from Explorer by hand.

## Inputs / Outputs

- **Inputs:** optional base PNG/JPG, TOML config, the published manifest (presets + latest version), flags (`--base`, `--name`, `--font`, `--accent`, `--secondary`, `--out`, `--watch`, `--tray`, `--preset`, `--list-presets`, `--manifest`, `--update`, `--version`, `--demo`, `--config`, `--write-config`)
- **Outputs:** sets the wallpaper via a PNG written to `%LOCALAPPDATA%\Phew Blue\wallpaper-info\` (or the pre-existing `%LOCALAPPDATA%\wallpaper-info\` on an upgraded install), alternating between `wallpaper.png` and `wallpaper-alt.png` (see Gotchas), or writes `--out <png>` previews. `docs/preview.png` is the tracked sample; root-level `preview*.png` is gitignored scratch.
- Defaults are phew-blue branding: accent `#0092CA`, secondary `#6A7078`, centered label = hostname (`-` hides it)

## Consumers

- **windows-setup** (`wallpaper-info.ps1`) — provisioning fetches the installer URL from the public manifest, verifies its sha256, and runs it `/VERYSILENT /PRESET=phew-blue`. The installer owns the binary, preset, background, and Startup tray entry, so the script no longer needs `gh` auth or a **brand** checkout. It still sets the lock/logon screen, which needs elevation.
- **website** — lists the project on `phew.blue/software` via a one-file content entry (`src/content/software/wallpaper-info.md`); the page pulls the description and latest release from GitHub itself. It hosts **nothing**: the manifest and backgrounds are release assets, so there is no cross-repo push, no `WEBSITE_PUSH_TOKEN`, and no cluster/HTTPRoute change.
- Deployed on the phew-blue Windows machines (e.g. LT-01-Windows) via that provisioning flow.

## Releases

Tag `v*` → `.github/workflows/release.yml` runs the tests, builds `wallpaper-info-windows-amd64.exe` **and** the Inno Setup installer (`iscc` under Wine, via the vendored `.github/actions/wine-inno`) on a GitHub-hosted runner, then generates `manifest.json`, stages the background PNGs under flat `background-<set>-<WxH>.png` names, and uploads everything to one GitHub release. **The release is the single source of truth** — clients read `releases/latest/download/manifest.json`, so nothing is published to another repo and no secrets are needed. The final step re-fetches the published manifest anonymously and HEADs every URL it advertises, so a release that would hand clients a dead preset fails instead. Runs on `ubuntu-latest` by default: GitHub refuses to dispatch a **public** repo's jobs to the `home-ops` self-hosted group (`allows_public_repositories=false`), and enabling that would expose the in-cluster runner and its RBAC to fork PRs. Set the repo variable `RUNNER` to target a self-hosted pool instead. The Go toolchain is fetched directly rather than via setup-go so it behaves identically on both.

## Gotchas

- Platform-specific files use `//go:build windows` / `//go:build !windows` — keep both sides of every pair in sync when changing signatures
- Win11 detection: the registry still reports "Windows 10" in `ProductName`; `osName()` rewrites it when `CurrentBuild >= 22000`
- `skipNic()` filters virtual adapters (Hyper-V, VMware, WSL, Docker) — extend the list rather than special-casing callers
- Public IP failures fall back to the last known value, then `"n/a"` — never block rendering on the network
- **`docs/preview.png` must only ever be regenerated with `--demo`.** It was previously a real render leaking a WAN IP, full name, and LAN topology. History was squashed and the GitHub repo deleted/recreated in Aug 2026 to purge it — branch rewrites alone were not enough, because tags and the `refs/pull/*` ref kept the old blobs fetchable. Never commit a render of a real machine
- The repo is **public** so the installer can fetch release assets anonymously (no `gh` auth on provisioned machines). Keep secrets and real host data out accordingly
- Config precedence is **defaults < preset < config file < explicit flags**; `flag.Visit` records which flags were explicit so `ApplyPreset` never overrides them
- `Info.Sig()` is facts-only; `Info.SigWith(cfg)` adds preset + layout. `--watch`/`--tray` must use `SigWith`, or a preset switch renders nothing
- Manifest/preset/background/update failures must always degrade (cache → local config), never fail a render
- In `presets/*.toml`, every top-level key (`backgrounds`, `background_set`, …) must stay **above** the `[layout]` header — a key written after a table header belongs to that table, which once silently published presets with no backgrounds at all. `TestRealPresetsParse` guards this
- **Never add `AppMutex` to `installer/wallpaper-info.iss`.** Inno checks it before `PrepareToInstall` and can only answer with a message box, so under `/VERYSILENT /SUPPRESSMSGBOXES` it defaults to Cancel: Setup exits 1 having installed nothing, turning every silent upgrade into a no-op. `PrepareToInstall`'s `taskkill` closes the running tray unattended instead
- **`PrepareToInstall` must `taskkill` both exe names.** The resident tray is `wallpaper-infow.exe`; killing only `wallpaper-info.exe` leaves it holding `{app}` open, and every upgrade silently installs nothing — the same failure mode as the `AppMutex` trap above, from the other direction
- **`DataDir()` must never move a directory.** For an upgraded install the data directory *is* the install directory — it holds the running exe, the catalogue and the uninstaller — so a rename races the installer, which reinstalls into the path it recorded. A version that did rename left LT-01 split: fonts and `wallpaper.png` under `Phew Blue\`, exe and catalogue still in the old folder, config referencing both. The rule that resolves it is to prefer the old directory whenever it exists, so a machine caught mid-split converges back onto the live install; fresh installs, having no old directory, get the new layout. `DefaultConfigPath` follows the same rule so a machine's config and data never end up in different layouts. See `notes/windows-app-layout.md`
- **A silent install with no `/PRESET` must not apply a default.** That is exactly what every self-update looks like, and `GetPreset` used to fall through to `phew-blue` — so a machine provisioned with a stick-only preset silently reverted the first time it updated itself (observed on LT-01: timeline-blue → phew-blue). `GetPresetArg` now returns no `--preset` at all when the config already names one, leaving the render to use what is saved. The wizard's choice still applies on an interactive install, guarded by `WizardSilent`
- **Update checks must read the published catalogue, never `app.Fetcher`.** A USB-provisioned machine's fetcher points at a local copy of the stick's catalogue, whose `latest.version` is frozen at whatever the stick was built with. Checking it would mean the machines provisioned from a stick are precisely the ones that never update again. `CheckAndUpdate` builds its own fetcher on `DefaultManifestURL`
- `SetWallpaper` alternates between `wallpaper.png` and `wallpaper-alt.png`, always writing the slot that is *not* currently set. Windows keeps the current wallpaper memory-mapped and rewriting that exact path fails with "a file with a user-mapped section open". `OurRenders()` lists both, and `baseCandidate` must reject both — guarding only one composites the panel onto its own render every other refresh
- `ScreenWidth()` calls `SetProcessDPIAware` first. Without it a scaled display reports virtualised pixels (2560x1440 at 125% reads as 2048x1152) and the nearest-resolution rule picks the wrong background
- Font *family* lookup must try the de-spaced name **with** a weight suffix: Google ships "Open Sans" as `OpenSans-Regular.ttf`. Miss that pairing and the render silently falls back to the 7x13 bitmap face — a tiny monospace panel. `TestResolveFontSpecFindsGoogleStyleFilenames` guards this
