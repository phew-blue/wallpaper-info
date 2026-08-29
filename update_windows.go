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

// CheckAndUpdate installs the newest release if the manifest advertises one. The sequence
// mirrors lexi's proven path: download, verify, run the installer silently, then exit so the
// installer can replace this running exe and restart it.
func CheckAndUpdate(app *App, userInitiated bool) error {
	// A scheduled check must not be answered from a day-old cache -- that is exactly the window
	// in which a new release appears. TTL 0 forces the network, and Get still falls back to the
	// cache when the network is unreachable, so an offline machine degrades to "no update"
	// rather than to an error.
	fresh := app.Fetcher
	fresh.TTL = 0
	m, err := fresh.Get()
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
	if err := downloadAsset(m.Latest.Setup, tmp); err != nil {
		return err
	}
	if err := exec.Command(tmp, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART").Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

// downloadAsset fetches a to dst and verifies its sha256. An unverified installer is never run.
func downloadAsset(a Asset, dst string) error {
	if a.URL == "" || a.SHA256 == "" {
		return fmt.Errorf("release asset has no url or sha256")
	}
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
		os.Remove(dst)
		return err
	}
	f.Close()

	if err := VerifySHA256(dst, a.SHA256); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}
