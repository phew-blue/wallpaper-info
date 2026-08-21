//go:build windows

package main

import "unsafe"

// kernel32 itself is declared in info_windows.go.
var (
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// ownsConsole reports whether this process is the only one attached to its console. That is
// true when Windows created the console for us (launched from Explorer, a shortcut, or the
// Startup folder) and false when we inherited the terminal the user is already sitting in.
func ownsConsole() bool {
	var pids [4]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}

// DetachConsole closes a console that Windows opened just for this process, so the resident
// tray/watch daemon leaves no console window on the desktop. A console inherited from the
// user's own terminal is deliberately left attached, so `--tray` run by hand still logs where
// they can see it.
//
// This is why the release build is a console binary rather than -H windowsgui: the GUI
// subsystem never blocks the calling shell, which made --list-presets and --version useless
// from a prompt.
func DetachConsole() {
	if ownsConsole() {
		procFreeConsole.Call()
	}
}
