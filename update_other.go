//go:build !windows

package main

import "errors"

// CheckAndUpdate: self-update is Windows-only; elsewhere the binary is built from source.
func CheckAndUpdate(app *App, userInitiated bool) error {
	return errors.New("self-update is only supported on Windows")
}
