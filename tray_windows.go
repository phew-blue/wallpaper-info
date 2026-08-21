//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"

	"github.com/getlantern/systray"
)

//go:embed assets/wallpaper-info.ico
var trayIcon []byte

// RunTray shows the tray icon and blocks until the user quits. systray.Run owns the thread,
// so everything interactive happens in onTrayReady's goroutines.
func RunTray(app *App) error {
	systray.Run(func() { onTrayReady(app) }, func() {})
	return nil
}

func onTrayReady(app *App) {
	systray.SetIcon(trayIcon)
	systray.SetTitle("wallpaper-info")
	systray.SetTooltip("wallpaper-info " + version)

	mRefresh := systray.AddMenuItem("Refresh now", "Re-render the wallpaper")
	mPresets := systray.AddMenuItem("Presets", "Switch preset")
	mUpdate := systray.AddMenuItem("Check for updates", "")
	mConfig := systray.AddMenuItem("Open config", "")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "")

	// The preset submenu comes from the manifest. A fetch failure leaves a disabled entry
	// rather than an empty menu, so the cause is visible on the desktop.
	if m, err := app.Fetcher.Get(); err == nil {
		for _, p := range m.Presets {
			p := p
			item := mPresets.AddSubMenuItemCheckbox(p.Name, p.Description, p.ID == app.Cfg.Preset)
			go func() {
				for range item.ClickedCh {
					if err := app.ApplyPresetByID(p.ID); err != nil {
						fmt.Fprintln(os.Stderr, "wallpaper-info:", err)
						continue
					}
					item.Check()
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

	// In tray mode the icon owns the refresh loop, replacing the bare --watch sleep loop.
	go app.Watch()
}
