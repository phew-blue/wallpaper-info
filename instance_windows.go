//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// InstanceMutexName is checked by the installer (AppMutex in wallpaper-info.iss) to detect a
// running copy. Changing it here means changing it there too.
const InstanceMutexName = "phew-blue-wallpaper-info"

var procCreateMutexW = kernel32.NewProc("CreateMutexW")

// HoldInstanceMutex creates the named mutex and deliberately never releases it: it exists for
// the lifetime of the resident process so the installer can tell we are running and close us
// before trying to replace our own .exe. Without this an upgrade over a running tray/watch
// process silently does nothing — Windows will not overwrite a locked binary.
func HoldInstanceMutex() {
	name, err := syscall.UTF16PtrFromString(InstanceMutexName)
	if err != nil {
		return
	}
	// Failure is not fatal: the installer also force-kills by image name.
	procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
}
