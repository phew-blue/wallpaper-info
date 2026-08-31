//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows/registry"
)

// Two icons, because the tray takes one and cannot follow the Windows theme by
// itself. The white glyph disappears on a light taskbar and the blue one is lost
// on a dark taskbar, so the set is chosen once at startup from
// SystemUsesLightTheme. The mark stays brand blue in both.

//go:embed assets/wallpaper-info.ico
var trayIconDark []byte

//go:embed assets/wallpaper-info-light.ico
var trayIconLight []byte

// trayIcon returns the one that will read against the current taskbar.
//
// SystemUsesLightTheme governs the taskbar and tray; AppsUseLightTheme governs
// application chrome and is a separate setting a user can set independently, so
// reading that one gives the right answer on most machines and the wrong one on
// anybody who mixes them. A missing key means dark, which is the Windows 11
// default.
//
// A theme changed after startup is not followed: Windows sends no usable
// notification to a systray-only process, and the tray already has a refresh
// loop that has no business polling the registry. Restarting picks it up.
func trayIcon() []byte {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE)
	if err != nil {
		return trayIconDark
	}
	defer k.Close()

	if v, _, err := k.GetIntegerValue("SystemUsesLightTheme"); err == nil && v == 1 {
		return trayIconLight
	}
	return trayIconDark
}

// RunTray shows the tray icon and blocks until the user quits. systray.Run owns the thread,
// so everything interactive happens in onTrayReady's goroutines.
func RunTray(app *App) error {
	systray.Run(func() { onTrayReady(app) }, func() {})
	return nil
}

func onTrayReady(app *App) {
	systray.SetIcon(trayIcon())
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
	go autoUpdate(app)
}

// autoUpdateDelay is how long the tray waits before its first update check. The tray starts at
// logon, when the network stack is often not up yet, so checking immediately would make a
// failed check the normal case rather than the exception.
const autoUpdateDelay = 5 * time.Minute

// autoUpdateInterval is how often a resident tray re-checks after that. A desktop that stays
// logged on for weeks otherwise never picks up a release unless someone opens the menu.
const autoUpdateInterval = 24 * time.Hour

// autoUpdate installs new releases in the background for the life of the tray process. A
// successful update never returns: CheckAndUpdate hands off to the installer and exits the
// process so the running exe can be replaced. Errors are logged and the loop continues -- an
// unreachable GitHub must not stop a desktop from refreshing its wallpaper.
func autoUpdate(app *App) {
	for time.Sleep(autoUpdateDelay); ; time.Sleep(autoUpdateInterval) {
		if err := CheckAndUpdate(app, false); err != nil {
			fmt.Fprintln(os.Stderr, "wallpaper-info: auto-update:", err)
		}
	}
}
