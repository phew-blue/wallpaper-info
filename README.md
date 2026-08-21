# wallpaper-info

Composites a small system-info panel (and an optional centered label) onto a background image and
sets it as the Windows desktop wallpaper. Self-contained single binary — the font is loaded from the
system (Open Sans if installed, else Segoe UI) and the background defaults to your current wallpaper,
so it runs anywhere with no assets. Shows **specs, not live usage**.

Bottom-right block: `user @ host`, OS, uptime, CPU (model + cores), RAM (total), disk (total),
LAN IP, WAN IP. Optional centered label (defaults to the hostname).

![preview](docs/preview.png)

## Build / run

```powershell
go build -o wallpaper-info.exe .
./wallpaper-info.exe            # composite onto current wallpaper, set it
```

## Customisation

Settings live in a **TOML config file**; flags override it for one-offs.

```
Windows : %APPDATA%\wallpaper-info\config.toml
macOS   : ~/.config/wallpaper-info/config.toml   (or $XDG_CONFIG_HOME)
Linux   : ~/.config/wallpaper-info/config.toml
```

Create one with your current settings, then edit it:
```powershell
./wallpaper-info.exe --name STUDIO-PC --accent "#40c4ff" --write-config
```
See `config.example.toml` for the keys. The config can also live in your dotfiles repo and be
symlinked here so it syncs across machines. `--config <path>` points at a specific file.

### Flags (override the config)

| Flag | Default | Meaning |
|---|---|---|
| `--base <png/jpg>` | current desktop wallpaper | background to draw onto (else a solid brand-dark canvas) |
| `--name <text>` | hostname | centered label; `-` to hide |
| `--font <name\|path>` | Open Sans → Segoe UI | font family (in Windows\Fonts) or a `.ttf` path |
| `--accent <hex>` | `#0092CA` | colour of `user @ host` |
| `--secondary <hex>` | `#6A7078` | colour of the detail lines |
| `--out <png>` | (unset) | write a preview PNG instead of setting the wallpaper |
| `--watch <minutes>` | `0` | re-render every N minutes (else run once) |
| `--config <path>` | per-OS config dir | use a specific config file |
| `--write-config` | — | save the current effective settings to the config file and exit |

Examples:
```powershell
./wallpaper-info.exe --out preview.png                          # preview, don't touch wallpaper
./wallpaper-info.exe --base "C:\path\bg.jpg" --name STUDIO-PC   # custom bg + label
./wallpaper-info.exe --accent "#40c4ff" --font "Cascadia Code"  # recolour + refont
./wallpaper-info.exe --watch 30                                 # keep IP/uptime fresh
```

Defaults are phew-blue branding; everything is overridable. A small GUI (file picker for the
background, colour pickers, run-at-startup) is planned.
