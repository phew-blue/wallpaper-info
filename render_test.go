package main

import (
	"strings"
	"testing"
)

func TestInfoLinesDefaultsToAllRows(t *testing.T) {
	got := infoLines(DemoInfo(), nil)
	joined := strings.Join(got, "\n")

	for _, want := range []string{"demo", "DEMO-PC", "Windows 11 Pro", "uptime", "i5-8265U", "16 GiB", "931 GiB", "Ethernet", "203.0.113.42"} {
		if !strings.Contains(joined, want) {
			t.Errorf("default rows missing %q; got:\n%s", want, joined)
		}
	}
}

func TestInfoLinesSelectsAndOrdersRows(t *testing.T) {
	got := infoLines(DemoInfo(), []string{"wan", "os"})
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(got), got)
	}
	if !strings.Contains(got[0], "203.0.113.42") {
		t.Errorf("first line = %q, want the WAN row first", got[0])
	}
	if !strings.Contains(got[1], "Windows 11 Pro") {
		t.Errorf("second line = %q, want the OS row second", got[1])
	}
}

func TestInfoLinesNicsExpandsToOnePerInterface(t *testing.T) {
	got := infoLines(DemoInfo(), []string{"nics"})
	if len(got) != 2 {
		t.Fatalf("got %d lines, want one per NIC: %v", len(got), got)
	}
}

func TestInfoLinesIgnoresUnknownRow(t *testing.T) {
	got := infoLines(DemoInfo(), []string{"os", "does-not-exist"})
	if len(got) != 1 {
		t.Fatalf("got %d lines, want unknown rows skipped: %v", len(got), got)
	}
}

func TestParseCorner(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Corner
		ok   bool
	}{
		{"bottom-right", BottomRight, true},
		{"top-left", TopLeft, true},
		{"sideways", BottomRight, false},
		{"", BottomRight, false},
	} {
		got, ok := ParseCorner(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseCorner(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
