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

Windows is the real target; non-Windows builds exist so development works anywhere.

## Build / Run

```bash
go build -o wallpaper-info.exe .          # mise pins the Go toolchain (.mise.toml)

./wallpaper-info.exe                      # composite onto current wallpaper, set it
./wallpaper-info.exe --out preview.png    # render a PNG instead (only option on non-Windows)
./wallpaper-info.exe --watch 30           # re-render every 30 min
```

No tests exist. Release builds use `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w"` (windowless, so the `--watch` daemon shows no console).

## Inputs / Outputs

- **Inputs:** optional base PNG/JPG, TOML config, flags (`--base`, `--name`, `--font`, `--accent`, `--secondary`, `--out`, `--watch`, `--config`, `--write-config`)
- **Outputs:** sets the wallpaper via a PNG written to `%LOCALAPPDATA%\wallpaper-info\wallpaper.png`, or writes `--out <png>` previews. `docs/preview.png` is the tracked sample; root-level `preview*.png` is gitignored scratch.
- Defaults are phew-blue branding: accent `#0092CA`, secondary `#6A7078`, centered label = hostname (`-` hides it)

## Consumers

- **windows-setup** (`wallpaper-info.ps1`) — provisioning downloads the latest release binary via `gh release download -R phew-blue/wallpaper-info`, picks the nearest-resolution logo-only wallpaper from the **brand** repo as `--base`, runs once, and installs a Startup-folder shortcut running `--watch 1`. It replaced the old Playwright-rendered per-host wallpapers.
- Deployed on the phew-blue Windows machines (e.g. LT-01-Windows) via that provisioning flow.

## Releases

Tag `v*` → `.github/workflows/release.yml` builds `wallpaper-info-windows-amd64.exe` on the `home-ops` self-hosted runner and attaches it to a GitHub release. The workflow downloads the Go toolchain directly to `RUNNER_TEMP` (setup-go is flaky on the runner's NFS tool cache).

## Gotchas

- Platform-specific files use `//go:build windows` / `//go:build !windows` — keep both sides of every pair in sync when changing signatures
- Win11 detection: the registry still reports "Windows 10" in `ProductName`; `osName()` rewrites it when `CurrentBuild >= 22000`
- `skipNic()` filters virtual adapters (Hyper-V, VMware, WSL, Docker) — extend the list rather than special-casing callers
- Public IP failures fall back to the last known value, then `"n/a"` — never block rendering on the network
