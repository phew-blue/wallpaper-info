package main

import (
	"strconv"
	"strings"
)

// NeedsUpdate reports whether latest is strictly newer than current. "dev" builds never update:
// a developer's local binary must not be replaced by a release.
func NeedsUpdate(current, latest string) bool {
	if current == "dev" || current == "" || latest == "" {
		return false
	}
	return compareSemver(strings.TrimPrefix(latest, "v"), strings.TrimPrefix(current, "v")) > 0
}

// compareSemver compares major.minor.patch numerically, so v0.10.0 beats v0.9.0.
func compareSemver(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		an, bn := 0, 0
		if i < len(as) {
			an, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bn, _ = strconv.Atoi(bs[i])
		}
		switch {
		case an > bn:
			return 1
		case an < bn:
			return -1
		}
	}
	return 0
}
