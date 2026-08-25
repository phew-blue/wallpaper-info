package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The current desktop wallpaper is almost always our own last render. Using it as a background
// would stack the info panel on itself at every refresh — the reason the old provisioning
// shortcut had to pin an explicit --base.
func TestBaseCandidateRejectsOurOwnOutput(t *testing.T) {
	out := `C:\Users\x\AppData\Local\wallpaper-info\wallpaper.png`
	// SetWallpaper alternates between two slots, so BOTH have to be rejected -- guarding only
	// the one we happened to write last would composite onto the other one every other refresh.
	alt := `C:\Users\x\AppData\Local\wallpaper-info\wallpaper-alt.png`
	ours := []string{out, alt}

	for _, tc := range []struct {
		name              string
		explicit, current string
		ours              []string
		want              string
	}{
		{"current is our own render", "", out, ours, ""},
		{"current is our alternate render", "", alt, ours, ""},
		{"case differs, still ours", "", `C:\USERS\X\APPDATA\LOCAL\WALLPAPER-INFO\WALLPAPER.PNG`, ours, ""},
		{"a real user wallpaper is fine", "", `C:\pics\beach.jpg`, ours, `C:\pics\beach.jpg`},
		{"explicit always wins", `C:\brand\bg.png`, out, ours, `C:\brand\bg.png`},
		{"explicit wins even if it is our render", out, out, ours, out},
		{"no current wallpaper", "", "", ours, ""},
		{"no output paths (non-Windows)", "", `C:\pics\beach.jpg`, nil, `C:\pics\beach.jpg`},
	} {
		if got := baseCandidate(tc.explicit, tc.current, tc.ours); got != tc.want {
			t.Errorf("%s: baseCandidate(%q,%q,%q) = %q, want %q",
				tc.name, tc.explicit, tc.current, tc.ours, got, tc.want)
		}
	}
}

func TestSamePath(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{`C:\x\wallpaper.png`, `C:\x\wallpaper.png`, true},
		{`C:\X\WALLPAPER.PNG`, `C:\x\wallpaper.png`, true}, // Windows is case-insensitive
		{`C:\x\base.png`, `C:\x\wallpaper.png`, false},
		{"", `C:\x\wallpaper.png`, false},
		{`C:\x\wallpaper.png`, "", false},
	} {
		if got := samePath(tc.a, tc.b); got != tc.want {
			t.Errorf("samePath(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLoadBaseHonoursExplicitPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bg.png")
	if err := os.WriteFile(p, mustPNG(t, 4, 3), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := LoadBase(p)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
		t.Errorf("got %v, want the 4x3 file we pointed at", img.Bounds())
	}
}

func TestLoadBaseFallsBackToSolidCanvas(t *testing.T) {
	img, err := LoadBase(filepath.Join(t.TempDir(), "missing.png"))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 1920 {
		t.Errorf("got %v, want the 1920x1080 solid fallback", img.Bounds())
	}
}
