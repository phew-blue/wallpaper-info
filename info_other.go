//go:build !windows

package main

import "runtime"

// Minimal fallbacks so the package builds on non-Windows (Windows is the real target).
func osName() string   { return runtime.GOOS }
func uptime() string   { return "" }
func cpuInfo() string  { return "" }
func ramInfo() string  { return "" }
func diskInfo() string { return "" }

// ScreenWidth: no display query off Windows; 1920 keeps background selection deterministic.
func ScreenWidth() int { return 1920 }
