//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetTickCount64 = kernel32.NewProc("GetTickCount64")
	procGlobalMemory   = kernel32.NewProc("GlobalMemoryStatusEx")
	procDiskSpace      = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func osName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows"
	}
	defer k.Close()
	product, _, _ := k.GetStringValue("ProductName")
	display, _, _ := k.GetStringValue("DisplayVersion")
	build, _, _ := k.GetStringValue("CurrentBuild")
	if n, _ := strconv.Atoi(build); n >= 22000 { // Win11 still reports "Windows 10" in ProductName
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	if display != "" {
		return fmt.Sprintf("%s (%s)", product, display)
	}
	return product
}

func uptime() string {
	r, _, _ := procGetTickCount64.Call()
	d := time.Duration(r) * time.Millisecond
	days := int(d.Hours()) / 24
	hrs := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hrs)
	case hrs > 0:
		return fmt.Sprintf("%dh %dm", hrs, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func cpuInfo() string {
	name := ""
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		name, _, _ = k.GetStringValue("ProcessorNameString")
		k.Close()
	}
	name = cleanCPU(name)
	cores := runtime.NumCPU()
	if name == "" {
		return fmt.Sprintf("%d cores", cores)
	}
	return fmt.Sprintf("%s · %d cores", name, cores)
}

func cleanCPU(s string) string {
	for _, junk := range []string{"(R)", "(TM)", "(r)", "(tm)", "CPU", "Processor"} {
		s = strings.ReplaceAll(s, junk, "")
	}
	if i := strings.Index(s, "@"); i > 0 {
		s = s[:i]
	}
	return strings.Join(strings.Fields(s), " ")
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func ramInfo() string {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemory.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return ""
	}
	return fmt.Sprintf("%.0f GiB RAM", float64(m.TotalPhys)/(1<<30))
}

func diskInfo() string {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	path, _ := syscall.UTF16PtrFromString(drive + `\`)
	var freeAvail, total, totalFree uint64
	if r, _, _ := procDiskSpace.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&freeAvail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree))); r == 0 {
		return ""
	}
	return fmt.Sprintf("%s  %.0f GiB · %.0f%% free", drive, float64(total)/(1<<30), float64(totalFree)/float64(total)*100)
}

// user32 itself is declared in wallpaper_windows.go.
var procGetSystemMetrics = user32.NewProc("GetSystemMetrics")

// ScreenWidth is the primary display's width in pixels, used to pick the nearest background.
func ScreenWidth() int {
	const smCXScreen = 0
	r, _, _ := procGetSystemMetrics.Call(uintptr(smCXScreen))
	if r == 0 {
		return 1920 // GetSystemMetrics failed (no display / session 0): assume 1080p
	}
	return int(r)
}
