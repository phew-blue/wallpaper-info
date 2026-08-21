package main

import "testing"

func TestNeedsUpdate(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"v0.3.0", "v0.4.0", true},
		{"v0.4.0", "v0.4.0", false},
		{"v0.4.1", "v0.4.0", false}, // never downgrade
		{"0.3.0", "v0.4.0", true},   // tolerate a missing v prefix
		{"dev", "v0.4.0", false},    // a dev build must never self-update
		{"v0.3.0", "", false},       // no version advertised: do nothing
		{"v0.9.0", "v0.10.0", true}, // numeric compare, not lexical
	} {
		if got := NeedsUpdate(tc.current, tc.latest); got != tc.want {
			t.Errorf("NeedsUpdate(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}
