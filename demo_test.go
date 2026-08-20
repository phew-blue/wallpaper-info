package main

import (
	"strings"
	"testing"
)

func TestDemoInfoIsSynthetic(t *testing.T) {
	got := DemoInfo()

	// Documentation-range addresses only (RFC 5737 / RFC 1918) — never a real WAN IP.
	if got.PublicIP != "203.0.113.42" {
		t.Errorf("PublicIP = %q, want the RFC 5737 documentation address", got.PublicIP)
	}
	if got.Host != "DEMO-PC" {
		t.Errorf("Host = %q, want DEMO-PC", got.Host)
	}
	if got.User != "demo" {
		t.Errorf("User = %q, want demo", got.User)
	}
	if len(got.Nics) == 0 {
		t.Fatal("Nics is empty; the preview should show at least one interface")
	}
	for _, n := range got.Nics {
		if !strings.HasPrefix(n.IP, "192.168.") {
			t.Errorf("NIC %s has IP %q, want a 192.168.0.0/16 private address", n.Name, n.IP)
		}
	}
}

func TestDemoInfoIsDeterministic(t *testing.T) {
	if DemoInfo().Sig() != DemoInfo().Sig() {
		t.Error("DemoInfo() is not deterministic; screenshots would not be reproducible")
	}
}
