# wallpaper-info

A single Go binary (flat `package main`, no subdirectories) that composites a system-info panel — `user @ host`, OS, uptime, CPU, RAM, disk, per-NIC LAN IPs, WAN IP — plus an optional centered label onto a background image and sets it as the Windows desktop wallpaper. Self-contained: the font is loaded from the system (Open Sans, else Segoe UI) and the background defaults to the current wallpaper, so it ships with no assets. Shows **specs, not live usage**.

---

## How It Works

- `main.go` — flag parsing, config merge (flags override config only when explicitly set via `flag.Visit`), the render-or-skip loop
- `config.go` — TOML config (`BurntSushi/toml`) at `%APPDATA%\wallpaper-info\config.toml` (Windows) or `~/.config/wallpaper-info/config.toml`; `--write-config` persists the current effective settings
- `info.go` — cross-platform facts (user, host, NICs, public IP via `api.ipify.org`, cached 15 min); `Info.Sig()` fingerprints everything shown so `--watch` skips re-rendering when nothing changed
- `info_windows.go` / `info_other.go` — platform facts (registry + kernel32 syscalls on Windows; non-Windows returns stubs)
- `base.go` — background selection: explicit `--base` path → current wallpaper → solid brand-dark canvas
- `render.go` — drawing via `golang.org/x/image` (opentype); sizes are vw-relative to match the brand wallpaper's CSS (`.machine` = 1.6vw, tag strip = 0.72vw)
- `wallpaper_windows.go` / `wallpaper_other.go` — `SystemParametersInfoW` set/get; non-Windows can only `--out` a PNG
- `app.go` — `App` holds run-time state (config, manifest fetcher) so the tray, the watch loop, and one-shot runs all drive the same `RenderOnce`/`ApplyPresetByID`
- `manifest.go` — fetches/parses the published catalogue; fresh cache → network → stale cache → give up. An unknown `schema` is ignored wholesale
- `preset.go` — applies a preset to `Config`, picks the nearest-resolution background, caches it content-addressed by sha256
- `demo.go` — `DemoInfo()`, the synthetic facts behind `--demo`
- `tray_windows.go` / `tray_other.go` — `getlantern/systray` tray icon (embedded `.ico`); non-Windows returns an error so the caller falls back to headless watch
- `update.go` + `update_windows.go` / `update_other.go` — `NeedsUpdate` version compare (platform-neutral, tested) and the silent installer hand-off
- `presets/*.toml` — authored preset sources; `tools/manifest` turns them into the published `manifest.json`
- `installer/wallpaper-info.iss` — per-user Inno Setup installer, compiled by `iscc` under Wine on the Linux runner

Windows is the real target; non-Windows builds exist so development works anywhere.

## Build / Run

```bash
go build -o wallpaper-info.exe .          # mise pins the Go toolchain (.mise.toml)

./wallpaper-info.exe                      # composite onto current wallpaper, set it
./wallpaper-info.exe --out preview.png    # render a PNG instead (only option on non-Windows)
./wallpaper-info.exe --watch 30           # re-render every 30 min
```

`go test ./...` covers the platform-independent logic: manifest parse/cache, preset application and precedence, background selection, config round-trip, render row selection, and version compare. Windows-only paths (tray, update, wallpaper set) cannot run in CI or in a Linux dev environment and are verified manually. Release builds use `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=$TAG"` (windowless, so the tray/watch daemon shows no console).

## Inputs / Outputs

- **Inputs:** optional base PNG/JPG, TOML config, the published manifest (presets + latest version), flags (`--base`, `--name`, `--font`, `--accent`, `--secondary`, `--out`, `--watch`, `--tray`, `--preset`, `--list-presets`, `--manifest`, `--update`, `--version`, `--demo`, `--config`, `--write-config`)
- **Outputs:** sets the wallpaper via a PNG written to `%LOCALAPPDATA%\wallpaper-info\wallpaper.png`, or writes `--out <png>` previews. `docs/preview.png` is the tracked sample; root-level `preview*.png` is gitignored scratch.
- Defaults are phew-blue branding: accent `#0092CA`, secondary `#6A7078`, centered label = hostname (`-` hides it)

## Consumers

- **windows-setup** (`wallpaper-info.ps1`) — provisioning fetches the installer URL from the public manifest, verifies its sha256, and runs it `/VERYSILENT /PRESET=phew-blue`. The installer owns the binary, preset, background, and Startup tray entry, so the script no longer needs `gh` auth or a **brand** checkout. It still sets the lock/logon screen, which needs elevation.
- **website** — serves `manifest.json` and `backgrounds/<preset>/<WxH>.png` from `public/wallpaper-info/`, reachable at both `wallpaper-info.phew.blue` and `phew.blue/wallpaper-info/`. The release workflow pushes the manifest there; `npm run sync:wallpaper-info` refreshes the backgrounds from a local brand checkout.
- Deployed on the phew-blue Windows machines (e.g. LT-01-Windows) via that provisioning flow.

## Releases

Tag `v*` → `.github/workflows/release.yml` builds `wallpaper-info-windows-amd64.exe` **and** the Inno Setup installer (`iscc` under Wine, via the vendored `.github/actions/wine-inno`) on the `home-ops` self-hosted runner, attaches both to a GitHub release, then generates `manifest.json` and pushes it to the **website** repo. Manifest generation is gated on proving both assets are anonymously downloadable, so it can never advertise a version whose upload failed. The workflow downloads the Go toolchain directly to `RUNNER_TEMP` (setup-go is flaky on the runner's NFS tool cache). Needs a `WEBSITE_PUSH_TOKEN` secret with `contents:write` on `phew-blue/website`.

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
