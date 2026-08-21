package main

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Render draws the centered label (if any) and the bottom-right info block onto a copy of bg.
func Render(bg image.Image, info Info, label, fontSpec, accentHex, secondaryHex string) image.Image {
	b := bg.Bounds()
	W, H := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(dst, dst.Bounds(), bg, b.Min, draw.Src)

	accent := parseHex(accentHex, color.RGBA{0x00, 0x92, 0xCA, 0xff})
	sec := parseHex(secondaryHex, color.RGBA{0x6A, 0x70, 0x78, 0xff})

	// Match the brand wallpaper's CSS sizes (vw = W/100): tag strip = 0.72vw, machine = 1.6vw.
	secSize := float64(W) * 0.0072
	reg := face(secSize, false, fontSpec)
	semi := face(secSize, true, fontSpec)

	// --- centered label (where the machine name used to be) ---
	if label != "" {
		nameSize := float64(W) * 0.016 // 1.6vw, matching the brand .machine style
		nameFace := face(nameSize, true, fontSpec)
		tracking := fixed.Int26_6(nameSize * 0.42 * 64) // 0.42em letter-spacing like .machine
		drawTracked(dst, nameFace, accent, strings.ToUpper(label), W/2, int(float64(H)*0.745), tracking)
	}

	// --- bottom-right info block ---
	type line struct {
		text string
		col  color.Color
		face font.Face
		gap  int
	}
	g := int(secSize * 0.8)
	lines := []line{
		{info.User + "  @  " + info.Host, accent, semi, 0},
		{info.OS, sec, reg, g},
		{"uptime " + info.Uptime, sec, reg, 0},
		{info.CPU, sec, reg, g},
		{info.RAM, sec, reg, 0},
		{info.Disk, sec, reg, 0},
	}
	// network: one labelled line per interface, then the public IP
	for i, nic := range info.Nics {
		gap := 0
		if i == 0 {
			gap = g
		}
		lines = append(lines, line{nic.Name + ":  " + nic.IP, sec, reg, gap})
	}
	wanGap := 0
	if len(info.Nics) == 0 {
		wanGap = g
	}
	lines = append(lines, line{"WAN:  " + info.PublicIP, sec, reg, wanGap})

	marginR := int(float64(W) * 0.030)
	// The brand tag strip is CSS `bottom: 7vh`, so its bottom edge is 0.07*H from the bottom.
	// Put our last line's text bottom on that same edge: baseline = H - 0.07H - descent.
	descent := float64(reg.Metrics().Descent) / 64.0
	marginB := int(float64(H)*0.07 + descent)
	lineH := int(secSize * 1.55)

	total := 0
	for _, ln := range lines {
		total += lineH + ln.gap
	}
	y := H - marginB - total + lineH
	for _, ln := range lines {
		y += ln.gap
		d := &font.Drawer{Dst: dst, Src: image.NewUniform(ln.col), Face: ln.face}
		w := d.MeasureString(ln.text)
		d.Dot = fixed.Point26_6{X: fixed.I(W-marginR) - w, Y: fixed.I(y)}
		d.DrawString(ln.text)
		y += lineH
	}
	return dst
}

// drawTracked draws s centered on cx at baseline, adding `tracking` between glyphs.
func drawTracked(dst draw.Image, fc font.Face, col color.Color, s string, cx, baseline int, tracking fixed.Int26_6) {
	runes := []rune(s)
	var total fixed.Int26_6
	for i, r := range runes {
		adv, _ := fc.GlyphAdvance(r)
		total += adv
		if i < len(runes)-1 {
			total += tracking
		}
	}
	x := fixed.I(cx) - total/2
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(col), Face: fc}
	for i, r := range runes {
		d.Dot = fixed.Point26_6{X: x, Y: fixed.I(baseline)}
		d.DrawString(string(r))
		adv, _ := fc.GlyphAdvance(r)
		x += adv
		if i < len(runes)-1 {
			x += tracking
		}
	}
}

// face loads a font: an explicit --font (path or family name) if given, else the brand default
// (Open Sans if installed, else Segoe UI), falling back to a built-in bitmap font so it never fails.
func face(size float64, bold bool, spec string) font.Face {
	var cands []string
	if spec != "" {
		cands = resolveFontSpec(spec, bold)
	} else {
		cands = fontCandidates(bold)
	}
	for _, p := range cands {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		f, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		if fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull}); err == nil {
			return fc
		}
	}
	return basicfont.Face7x13
}

// fontDirs are searched in order: per-user fonts (no-admin installs) then the system Fonts dir.
func fontDirs() []string {
	var dirs []string
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		dirs = append(dirs, filepath.Join(la, "Microsoft", "Windows", "Fonts"))
	}
	win := os.Getenv("WINDIR")
	if win == "" {
		win = `C:\Windows`
	}
	return append(dirs, filepath.Join(win, "Fonts"))
}

func inDirs(names ...string) []string {
	var out []string
	for _, d := range fontDirs() {
		for _, n := range names {
			out = append(out, filepath.Join(d, n))
		}
	}
	return out
}

func fontCandidates(bold bool) []string {
	if bold {
		return inDirs("OpenSans-SemiBold.ttf", "seguisb.ttf", "segoeui.ttf")
	}
	return inDirs("OpenSans-Regular.ttf", "segoeui.ttf")
}

// resolveFontSpec turns a --font value into candidate file paths: a direct .ttf path, or a family
// name looked up in the font dirs (trying a SemiBold variant first for bold text).
func resolveFontSpec(spec string, bold bool) []string {
	if st, err := os.Stat(spec); err == nil && !st.IsDir() {
		return []string{spec}
	}
	nospace := strings.ReplaceAll(spec, " ", "")
	var names []string
	if bold {
		names = append(names, spec+"-SemiBold.ttf", nospace+"b.ttf", nospace+"sb.ttf")
	}
	names = append(names, spec+".ttf", nospace+".ttf", spec+"-Regular.ttf")
	return inDirs(names...)
}

func parseHex(s string, def color.RGBA) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return def
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return def
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}
