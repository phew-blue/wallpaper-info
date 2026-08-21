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

// Corner selects which edge the info block is anchored to.
type Corner int

const (
	BottomRight Corner = iota // current, and the brand-wallpaper default
	BottomLeft
	TopRight
	TopLeft
)

// ParseCorner maps a config string onto a Corner. The bool reports whether the string was known;
// an unknown value must fall back to the default rather than silently moving the panel.
func ParseCorner(s string) (Corner, bool) {
	switch s {
	case "bottom-right":
		return BottomRight, true
	case "bottom-left":
		return BottomLeft, true
	case "top-right":
		return TopRight, true
	case "top-left":
		return TopLeft, true
	}
	return BottomRight, false
}

func (c Corner) right() bool { return c == BottomRight || c == TopRight }
func (c Corner) top() bool   { return c == TopLeft || c == TopRight }

// DefaultRows is the full row set, in the order the panel has always rendered them.
var DefaultRows = []string{"user", "os", "uptime", "cpu", "ram", "disk", "nics", "wan"}

// RenderOpts carries everything about how the panel looks, so Render keeps a two-argument
// data/one-argument-options shape as more presentation knobs arrive.
type RenderOpts struct {
	Label     string // centered label ("" = none)
	Font      string // family name or .ttf path
	Accent    string // hex
	Secondary string // hex
	Corner    Corner
	Rows      []string // nil or empty = DefaultRows
}

// infoRow is one rendered line. group marks the start of a visual group, which gets a gap
// above it — reproducing the original spacing before the OS, hardware, and network blocks.
type infoRow struct {
	text  string
	group bool
}

// infoRows turns the requested rows into display lines. Unknown row names are skipped so a
// newer manifest naming a row this build does not know cannot blank the panel.
func infoRows(info Info, rows []string) []infoRow {
	if len(rows) == 0 {
		rows = DefaultRows
	}
	var out []infoRow
	for _, r := range rows {
		switch r {
		case "user":
			out = append(out, infoRow{info.User + "  @  " + info.Host, false})
		case "os":
			out = append(out, infoRow{info.OS, true})
		case "uptime":
			out = append(out, infoRow{"uptime " + info.Uptime, false})
		case "cpu":
			out = append(out, infoRow{info.CPU, true})
		case "ram":
			out = append(out, infoRow{info.RAM, false})
		case "disk":
			out = append(out, infoRow{info.Disk, false})
		case "nics":
			for i, n := range info.Nics {
				out = append(out, infoRow{n.Name + ":  " + n.IP, i == 0})
			}
		case "wan":
			// The WAN line opens the network group only when no NIC lines preceded it.
			out = append(out, infoRow{"WAN:  " + info.PublicIP, len(info.Nics) == 0})
		}
	}
	return out
}

// infoLines is the text-only view of infoRows, for callers that do not care about spacing.
func infoLines(info Info, rows []string) []string {
	rs := infoRows(info, rows)
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.text)
	}
	return out
}

// Render draws the centered label (if any) and the info block onto a copy of bg.
func Render(bg image.Image, info Info, opts RenderOpts) image.Image {
	label, fontSpec, accentHex, secondaryHex := opts.Label, opts.Font, opts.Accent, opts.Secondary
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
	var lines []line
	for i, r := range infoRows(info, opts.Rows) {
		gap := 0
		if r.group && i > 0 { // no gap above the very first line
			gap = g
		}
		// The first line is the accented user@host header; the rest are secondary detail.
		if i == 0 {
			lines = append(lines, line{r.text, accent, semi, gap})
			continue
		}
		lines = append(lines, line{r.text, sec, reg, gap})
	}

	marginX := int(float64(W) * 0.030)
	// The brand tag strip is CSS `bottom: 7vh`, so its bottom edge is 0.07*H from the bottom.
	// Put our last line's text bottom on that same edge: baseline = H - 0.07H - descent.
	descent := float64(reg.Metrics().Descent) / 64.0
	marginY := int(float64(H)*0.07 + descent)
	lineH := int(secSize * 1.55)

	total := 0
	for _, ln := range lines {
		total += lineH + ln.gap
	}
	// Bottom anchors compute the first baseline back from the bottom edge; top anchors start at
	// the top margin and advance downwards.
	y := H - marginY - total + lineH
	if opts.Corner.top() {
		y = marginY + lineH
	}
	for _, ln := range lines {
		y += ln.gap
		d := &font.Drawer{Dst: dst, Src: image.NewUniform(ln.col), Face: ln.face}
		x := fixed.I(marginX) // left-anchored
		if opts.Corner.right() {
			x = fixed.I(W-marginX) - d.MeasureString(ln.text)
		}
		d.Dot = fixed.Point26_6{X: x, Y: fixed.I(y)}
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
	// Google's font files drop the space AND carry a weight suffix ("OpenSans-Regular.ttf"), so
	// the de-spaced name has to be tried with the suffix too. Without that pairing, --font
	// "Open Sans" found nothing on a machine that had Open Sans installed and silently fell back
	// to the 7x13 bitmap face -- a tiny monospace panel that scaled with nothing.
	nospace := strings.ReplaceAll(spec, " ", "")
	var names []string
	if bold {
		names = append(names, spec+"-SemiBold.ttf", nospace+"-SemiBold.ttf", nospace+"b.ttf", nospace+"sb.ttf")
	}
	names = append(names, spec+".ttf", nospace+".ttf", spec+"-Regular.ttf", nospace+"-Regular.ttf")
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
