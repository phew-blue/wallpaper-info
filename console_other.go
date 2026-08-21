//go:build !windows

package main

// DetachConsole: nothing to detach off Windows.
func DetachConsole() {}
