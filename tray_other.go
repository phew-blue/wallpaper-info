//go:build !windows

package main

import "errors"

// RunTray: the tray is Windows-only. Callers fall back to the headless watch loop.
func RunTray(app *App) error { return errors.New("tray mode is only supported on Windows") }
