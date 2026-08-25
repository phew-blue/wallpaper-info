//go:build windows

package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	user32  = syscall.NewLazyDLL("user32.dll")
	procSPI = user32.NewProc("SystemParametersInfoW")
)

const (
	spiGetDeskWallpaper = 0x0073
	spiSetDeskWallpaper = 0x0014
	spifUpdateINIFile   = 0x01
	spifSendChange      = 0x02
)

func currentWallpaperPath() string {
	buf := make([]uint16, 520)
	r, _, _ := procSPI.Call(spiGetDeskWallpaper, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])), 0)
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// OurRenders lists every path SetWallpaper may write to. LoadBase must never composite onto
// any of them: stacking the info panel on our own previous render doubles the text at every
// refresh.
//
// There are two, because Windows keeps the *current* wallpaper file memory-mapped and rewriting
// that exact path fails with "the requested operation cannot be performed on a file with a
// user-mapped section open". Alternating means the slot we write is never the mapped one.
func OurRenders() []string {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "wallpaper-info")
	return []string{
		filepath.Join(dir, "wallpaper.png"),
		filepath.Join(dir, "wallpaper-alt.png"),
	}
}

func SetWallpaper(img image.Image) error {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "wallpaper-info")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	current := currentWallpaperPath()
	out := OurRenders()[0]
	if samePath(current, out) {
		out = OurRenders()[1]
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	f.Close()

	p, _ := syscall.UTF16PtrFromString(out)
	r, _, err := procSPI.Call(spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(p)), spifUpdateINIFile|spifSendChange)
	if r == 0 {
		return err
	}
	return nil
}
