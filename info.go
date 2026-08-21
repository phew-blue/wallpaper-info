package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"
)

// NIC is a network interface with its IPv4 address, e.g. {"Wi-Fi", "192.168.1.20"}.
type NIC struct {
	Name, IP string
}

// Info is the set of (cheap, static-ish) facts shown on the wallpaper. No live usage metrics.
type Info struct {
	User, Host, OS, Uptime, CPU, RAM, Disk, PublicIP string
	Nics                                             []NIC
}

func Gather() Info {
	return Info{
		User:     currentUser(),
		Host:     hostname(),
		OS:       osName(),   // platform-specific
		Uptime:   uptime(),   // platform-specific
		CPU:      cpuInfo(),  // platform-specific
		RAM:      ramInfo(),  // platform-specific
		Disk:     diskInfo(), // platform-specific
		Nics:     localNics(),
		PublicIP: publicIP(),
	}
}

// localNics lists up, non-loopback interfaces that have a real IPv4 address, labelled by their
// friendly name (Wi-Fi, Ethernet, ...). Noisy host-virtual adapters are filtered out.
func localNics() []NIC {
	var out []NIC
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || skipNic(ifc.Name) {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, NIC{Name: ifc.Name, IP: ip.String()})
			break // one IPv4 per interface
		}
	}
	return out
}

func skipNic(name string) bool {
	low := strings.ToLower(name)
	for _, s := range []string{"loopback", "vethernet", "hyper-v", "vmware", "virtualbox", "wsl", "docker"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	n := u.Username
	if i := strings.LastIndexAny(n, `\/`); i >= 0 {
		n = n[i+1:]
	}
	return n
}

func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "unknown"
	}
	return h
}

// Sig is a fingerprint of everything shown, so --watch can skip re-setting the wallpaper when
// nothing changed (cheap polling at short intervals).
func (i Info) Sig() string {
	s := i.User + "|" + i.Host + "|" + i.OS + "|" + i.Uptime + "|" + i.CPU + "|" + i.RAM + "|" + i.Disk + "|" + i.PublicIP
	for _, n := range i.Nics {
		s += "|" + n.Name + "=" + n.IP
	}
	return s
}

// public IP is cached so a short --watch interval doesn't hammer the network (or hang when offline).
var (
	pubMu  sync.Mutex
	pubVal string
	pubAt  time.Time
)

func publicIP() string {
	pubMu.Lock()
	defer pubMu.Unlock()
	if pubVal != "" && time.Since(pubAt) < 15*time.Minute {
		return pubVal
	}
	cl := http.Client{Timeout: 4 * time.Second}
	resp, err := cl.Get("https://api.ipify.org")
	if err != nil {
		if pubVal != "" {
			return pubVal // keep last known if a refresh fails
		}
		return "n/a"
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	if ip := strings.TrimSpace(string(b)); ip != "" {
		pubVal, pubAt = ip, time.Now()
		return ip
	}
	if pubVal != "" {
		return pubVal
	}
	return "n/a"
}
