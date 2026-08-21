// Command wallpaper-info composites a system-info block (and an optional centered label) onto a
// background image and sets it as the desktop wallpaper. Self-contained: the font comes from the
// system and the background defaults to the current desktop wallpaper, so it runs anywhere with no
// assets. Settings come from a TOML config file; flags override it for one-offs.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	configPath := flag.String("config", "", "config file path (default: per-OS config dir)")
	writeConfig := flag.Bool("write-config", false, "write the current effective settings to the config file and exit")
	base := flag.String("base", "", "background image (default: current desktop wallpaper, else solid)")
	name := flag.String("name", "", "centered label (default: hostname; '-' to hide)")
	out := flag.String("out", "", "write the result PNG here instead of setting the wallpaper (preview)")
	watch := flag.Int("watch", 0, "refresh every N minutes (0 = run once)")
	demo := flag.Bool("demo", false, "render synthetic sample data instead of this machine's facts (for docs)")
	preset := flag.String("preset", "", "apply a named preset from the manifest")
	listPresets := flag.Bool("list-presets", false, "list presets available in the manifest and exit")
	manifestURL := flag.String("manifest", "", "manifest URL (default "+DefaultManifestURL+")")
	showVersion := flag.Bool("version", false, "print the version and exit")
	tray := flag.Bool("tray", false, "run resident with a system-tray icon (Windows; implies --watch)")
	update := flag.Bool("update", false, "download and install the latest release, then exit")
	fontSpec := flag.String("font", "", "font family name or .ttf path (default: Open Sans if installed, else Segoe UI)")
	accent := flag.String("accent", "", "accent colour hex (default #0092CA)")
	secondary := flag.String("secondary", "", "secondary text colour hex (default #6A7078)")
	flag.Parse()

	cfgFile := *configPath
	if cfgFile == "" {
		cfgFile = DefaultConfigPath()
	}
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wallpaper-info: config:", err)
		os.Exit(1)
	}

	// Flags override config only when explicitly set on the command line. The same pass records
	// which flags were explicit, so a preset can fill everything else without clobbering them.
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		explicit[f.Name] = true
		switch f.Name {
		case "preset":
			cfg.Preset = *preset
		case "base":
			cfg.Base = *base
		case "name":
			cfg.Name = *name
		case "font":
			cfg.Font = *fontSpec
		case "accent":
			cfg.Accent = *accent
		case "secondary":
			cfg.Secondary = *secondary
		case "watch":
			cfg.Watch = *watch
		}
	})

	if *showVersion {
		fmt.Println(version)
		return
	}

	// Presets are best-effort: a machine with no network must still render from local config.
	if cfg.Preset != "" || *listPresets {
		m, err := NewManifestFetcher(*manifestURL).Get()
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
			if p, ok := m.Preset(cfg.Preset); !ok {
				fmt.Fprintf(os.Stderr, "wallpaper-info: preset %q not in manifest, using local config\n", cfg.Preset)
			} else {
				cfg = ApplyPreset(cfg, p, explicit)
				if !explicit["base"] && cfg.Base == "" {
					if bg, ok := PickBackground(p.Backgrounds, ScreenWidth()); ok {
						path, err := EnsureBackground(bg, &http.Client{Timeout: 60 * time.Second})
						if err != nil {
							fmt.Fprintln(os.Stderr, "wallpaper-info: background:", err)
						} else {
							cfg.Base = path
						}
					}
				}
			}
		}
	}

	if *writeConfig {
		if err := SaveConfig(cfgFile, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", cfgFile)
		return
	}

	// --tray is a resident mode, so it implies a refresh interval: without one, Watch() returns
	// immediately and the tray sits there showing a wallpaper that never updates. 1 minute
	// matches what the old provisioning shortcut used, and the Sig check makes it cheap.
	if *tray && cfg.Watch == 0 {
		cfg.Watch = 1
	}

	app := &App{
		Cfg:      cfg,
		CfgPath:  cfgFile,
		Demo:     *demo,
		Out:      *out,
		Explicit: explicit,
		Fetcher:  NewManifestFetcher(*manifestURL),
	}

	if *update {
		if err := CheckAndUpdate(app, true); err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info: update:", err)
			os.Exit(1)
		}
		return
	}

	if err := app.RenderOnce(false); err != nil {
		fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
		os.Exit(1)
	}

	// Past this point the process is resident, so: drop a console Windows opened just for us
	// (Startup shortcut, Explorer) while leaving the user's own terminal alone, and hold the
	// instance mutex so an installer can tell we are running and close us before replacing
	// our own .exe.
	if *tray || cfg.Watch > 0 {
		DetachConsole()
		HoldInstanceMutex()
	}

	if *tray {
		// A shell that will not host a tray icon must not cost the machine its wallpaper:
		// fall through to the headless watch loop instead of exiting.
		if err := RunTray(app); err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info: tray unavailable, running headless:", err)
		} else {
			return
		}
	}
	app.Watch()
}

// version is stamped at build time: -ldflags "-X main.version=$TAG". "dev" never self-updates.
var version = "dev"
