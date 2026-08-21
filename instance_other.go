//go:build !windows

package main

// HoldInstanceMutex: single-instance detection is a Windows installer concern only.
func HoldInstanceMutex() {}
