package brand

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"time"
)

//go:generate go run ./genart

// artSeed fixes the greens in every asset: the day the banner design was approved.
const artSeed = 20261016

// palette holds one GitHub theme's colours: text, secondary text, and greens.
type palette struct {
	fg, muted string
	green     [5]string
}

func themePalette(dark bool) palette {
	p := palette{fg: "#1f2328", muted: "#59636e"}
	if dark {
		p = palette{fg: "#f0f6fc", muted: "#9198a1"}
	}
	for i, c := range Greens {
		p.green[i] = c.Light
		if dark {
			p.green[i] = c.Dark
		}
	}
	return p
}

// cell is one square of a contribution grid, span units wide and tall.
type cell struct {
	x, y, span int
	color      string
}

// glyphs is a 5x9 pixel face for the letters of "greenbridge": rows 0-1
// ascenders, 2-6 x-height, 7-8 the descender of g.
var glyphs = map[rune][9]string{
	'b': {"#....", "#....", "####.", "#...#", "#...#", "#...#", "####.", ".....", "....."},
	'd': {"....#", "....#", ".####", "#...#", "#...#", "#...#", ".####", ".....", "....."},
	'e': {".....", ".....", ".###.", "#...#", "#####", "#....", ".###.", ".....", "....."},
	'g': {".....", ".....", ".####", "#...#", "#...#", "#...#", ".####", "....#", ".###."},
	'i': {"..#..", ".....", ".##..", "..#..", "..#..", "..#..", ".###.", ".....", "....."},
	'n': {".....", ".....", "####.", "#...#", "#...#", "#...#", "#...#", ".....", "....."},
	'r': {".....", ".....", "#.##.", "##..#", "#....", "#....", "#....", ".....", "....."},
}

// wordmark sets "green" in the text colour and "bridge" in green, as the
// prototype's lockup does. It is 65 cells wide and 9 tall.
func wordmark(p palette, ox, oy int) []cell {
	var out []cell
	for i, r := range "greenbridge" {
		c := p.fg
		if i >= len("green") {
			c = p.green[3]
		}
		for y, row := range glyphs[r] {
			for x, px := range row {
				if px == '#' {
					out = append(out, cell{ox + i*6 + x, oy + y, 1, c})
				}
			}
		}
	}
	return out
}

// bridgeCells draws a bridge one cell per pixel, each span units square.
func bridgeCells(b Bridge, seed uint32, p palette, ox, oy, span int) []cell {
	var g grid
	b.draw(&g)
	lv := busy(seed)
	var out []cell
	for y := range H {
		for x := range W {
			if g[y][x] {
				out = append(out, cell{ox + x*span, oy + y*span, span, p.green[lv[y][x]]})
			}
		}
	}
	return out
}

// The hero is the Golden Gate over the wordmark, in grid units. The bridge's
// squares span 2 units so it leads; the 65-unit wordmark sits centred under it.
const heroW, heroH = 2 * W, 2*H + 4 + 9

func hero(p palette) []cell {
	return append(bridgeCells(Bridges[0], artSeed, p, 0, 0, 2), wordmark(p, (heroW-65)/2, 2*H+4)...)
}

func svgCells(sb *strings.Builder, cs []cell, size, ox, oy int) {
	for _, c := range cs {
		s := c.span * size
		gap := max(1, s/5)
		fmt.Fprintf(sb, `<rect x="%d" y="%d" width="%d" height="%d" rx="%d" fill="%s"/>`+"\n",
			ox+c.x*size, oy+c.y*size, s-gap, s-gap, gap, c.color)
	}
}

func heroSVG(dark bool) []byte {
	const size = 8
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n",
		heroW*size, heroH*size, heroW*size, heroH*size)
	sb.WriteString("<title>greenbridge</title>\n")
	svgCells(&sb, hero(themePalette(dark)), size, 0, 0)
	sb.WriteString("</svg>\n")
	return []byte(sb.String())
}

var xmlEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// socialBg is GitHub's dark canvas.
const socialBg = "#0d1117"

// socialPNG is GitHub's 1280x640 social preview: the dark hero over a
// mirror ledger on GitHub's dark canvas.
func socialPNG() []byte {
	const w, h, size = 1280, 640, 12
	img := socialImage(w, h, size)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err) // encoding an in-memory RGBA cannot fail
	}
	return buf.Bytes()
}

func socialImage(w, h, size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	ox, oy := (w-heroW*size)/2, (h-heroH*size)/2
	cells := hero(themePalette(true))
	drawLedger(img, newCoverage(w, h, cells, size, ox, oy))
	for _, c := range cells {
		sq := c.span*size - c.span*size/5
		col := hexColor(c.color)
		x0, y0 := ox+c.x*size, oy+c.y*size
		for dy := range sq {
			for dx := range sq {
				corner := (dx == 0 || dx == sq-1) && (dy == 0 || dy == sq-1)
				if !corner {
					img.Set(x0+dx, y0+dy, col)
				}
			}
		}
	}
	return img
}

// The background of the social preview is a mirror ledger: rows of
// "gb/1 <id>  <time>", the whole payload of a mirror commit, with synthetic
// values drawn from artSeed. A dark halo follows the hero: nothing within
// ledgerHaloHard px of a hero square, full strength past ledgerHaloSoft.
// Amounts are integers with 256 as 1.0, so every platform draws the same
// pixels (arm64 may fuse float maths into FMA).
const (
	ledgerCellW, ledgerCellH       = 12, 18
	ledgerScale                    = 2 // device px per glyph pixel
	ledgerHaloHard, ledgerHaloSoft = 16, 48
	ledgerPrefix                   = "gb/1"
	ledgerMuted                    = "#8b949e"
	ledgerPrefixA, ledgerIDA       = 87, 159 // opacity of the prefix and the id
	ledgerTimeA                    = 113     // opacity of the timestamp
	ledgerTime                     = "2006-01-02 15:04"
)

// ledgerEpoch is the synthetic ledger's first timestamp.
var ledgerEpoch = time.Date(2026, 9, 2, 8, 41, 0, 0, time.UTC)

// ledgerEntry is one synthetic mirror commit: an id, a time, and a strength
// k/256 that varies its opacity so the ledger does not read as a flat block.
type ledgerEntry struct {
	id string
	at time.Time
	k  int
}

func (e ledgerEntry) String() string {
	return ledgerPrefix + " " + e.id + "  " + e.at.Format(ledgerTime)
}

// ledgerEntries draws n entries from seed alone, minutes apart and in order.
func ledgerEntries(seed uint32, n int) []ledgerEntry {
	const hex = "0123456789abcdef"
	r := mulberry(seed)
	at := ledgerEpoch
	out := make([]ledgerEntry, n)
	for i := range out {
		k := 205 + r.intn(52)
		id := make([]byte, 12)
		for j := range id {
			id[j] = hex[r.intn(16)]
		}
		at = at.Add(time.Duration(4+r.intn(187)) * time.Minute)
		out[i] = ledgerEntry{string(id), at, k}
	}
	return out
}

// ledgerCap is how many entries a cols by rows ledger uses at most.
func ledgerCap(cols, rows int) int {
	return rows * (cols/len(ledgerEntry{}.String()+"   ") + 2)
}

// ledgerCell is one character of the ledger: a rune (0 for none), its colour,
// and its opacity at full strength.
type ledgerCell struct {
	ch    rune
	color string
	alpha int
}

// ledger lays entries out row by row, each row starting part-way into an
// entry so the columns bleed off both edges.
func ledger(cols, rows int, es []ledgerEntry) [][]ledgerCell {
	g := make([][]ledgerCell, rows)
	put := func(x, y int, s, color string, alpha int) int {
		for _, r := range s {
			if x >= 0 && x < cols && r != ' ' {
				g[y][x] = ledgerCell{r, color, alpha}
			}
			x++
		}
		return x
	}
	next := 0
	green := themePalette(true).green[3]
	for y := range g {
		g[y] = make([]ledgerCell, cols)
		for x := -9; x < cols; next++ {
			e := es[next]
			x = put(x, y, ledgerPrefix, ledgerMuted, ledgerPrefixA*e.k/256) + 1
			x = put(x, y, e.id, green, ledgerIDA*e.k/256) + 2
			x = put(x, y, e.at.Format(ledgerTime), ledgerMuted, ledgerTimeA*e.k/256) + 3
		}
	}
	return g
}

// ledgerFont is a 5x8 face for the ledger: rows 0-6 down to the baseline,
// row 7 the descender of g. Lowercase letters sit on rows 2-6 and g drops
// below the baseline, so it cannot pass for a 9 even at the 400x200 unfurl.
const ledgerBaseline = 6

var ledgerFont = map[rune][8]string{
	'0': {".###.", "#...#", "#..##", "#.#.#", "##..#", "#...#", ".###.", "....."},
	'1': {"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###.", "....."},
	'2': {".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####", "....."},
	'3': {"#####", "...#.", "..#..", "...#.", "....#", "#...#", ".###.", "....."},
	'4': {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#.", "....."},
	'5': {"#####", "#....", "####.", "....#", "....#", "#...#", ".###.", "....."},
	'6': {"..##.", ".#...", "#....", "####.", "#...#", "#...#", ".###.", "....."},
	'7': {"#####", "....#", "...#.", "..#..", ".#...", ".#...", ".#...", "....."},
	'8': {".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###.", "....."},
	'9': {".###.", "#...#", "#...#", ".####", "....#", "...#.", ".##..", "....."},
	'a': {".....", ".....", ".###.", "....#", ".####", "#...#", ".####", "....."},
	'b': {"#....", "#....", "####.", "#...#", "#...#", "#...#", "####.", "....."},
	'c': {".....", ".....", ".###.", "#....", "#....", "#...#", ".###.", "....."},
	'd': {"....#", "....#", ".####", "#...#", "#...#", "#...#", ".####", "....."},
	'e': {".....", ".....", ".###.", "#...#", "#####", "#....", ".###.", "....."},
	'f': {"..##.", ".#..#", ".#...", "###..", ".#...", ".#...", ".#...", "....."},
	'g': {".....", ".....", ".####", "#...#", "#...#", ".####", "....#", "####."},
	'/': {"....#", "....#", "...#.", "..#..", ".#...", "#....", "#....", "....."},
	'-': {".....", ".....", ".....", ".###.", ".....", ".....", ".....", "....."},
	':': {".....", ".##..", ".##..", ".....", ".##..", ".##..", ".....", "....."},
}

// drawLedger draws the ledger over GitHub's dark canvas, fading each cell by
// its distance to the hero.
func drawLedger(img *image.RGBA, hero coverage) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	bg := hexColor(socialBg)
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, bg)
		}
	}
	cols, rows := w/ledgerCellW, h/ledgerCellH
	padX, padY := (w-cols*ledgerCellW)/2, (h-rows*ledgerCellH)/2
	g := ledger(cols, rows, ledgerEntries(artSeed, ledgerCap(cols, rows)))
	// halo returns 0-256: 0 within the hard radius of the hero, easing up to
	// 256 at the soft radius.
	halo := func(x0, y0, x1, y1 int) int {
		for d := 0; d <= ledgerHaloSoft; d += 2 {
			if hero.count(x0-d, y0-d, x1+d, y1+d) > 0 {
				if d < ledgerHaloHard {
					return 0
				}
				t := (d - ledgerHaloHard) * 256 / (ledgerHaloSoft - ledgerHaloHard)
				return t * t * (768 - 2*t) >> 16
			}
		}
		return 256
	}
	const gw, gh = 5 * ledgerScale, 7 * ledgerScale
	for cy, row := range g {
		for cx, c := range row {
			if c.ch == 0 {
				continue
			}
			x0, y0 := padX+cx*ledgerCellW, padY+cy*ledgerCellH
			a := c.alpha * halo(x0, y0, x0+ledgerCellW, y0+ledgerCellH) / 256
			if a == 0 {
				continue
			}
			glyph, ok := ledgerFont[c.ch]
			if !ok {
				panic(fmt.Sprintf("no ledger glyph for %q", c.ch)) // the ledger's alphabet is fixed in this file
			}
			col := hexColor(c.color)
			gx, gy := x0+(ledgerCellW-gw)/2, y0+(ledgerCellH-gh)/2
			for py, line := range glyph {
				for px, on := range line {
					if on != '#' {
						continue
					}
					for sy := range ledgerScale {
						for sx := range ledgerScale {
							x, y := gx+px*ledgerScale+sx, gy+py*ledgerScale+sy
							img.SetRGBA(x, y, mix(img.RGBAAt(x, y), col, a))
						}
					}
				}
			}
		}
	}
}

// coverage counts hero pixels in any rectangle in constant time.
type coverage struct {
	w, h int
	sum  []int // summed-area table, (w+1) by (h+1)
}

func newCoverage(w, h int, cells []cell, size, ox, oy int) coverage {
	on := make([]int, w*h)
	for _, c := range cells {
		sq := c.span*size - c.span*size/5
		for dy := range sq {
			for dx := range sq {
				on[(oy+c.y*size+dy)*w+ox+c.x*size+dx] = 1
			}
		}
	}
	cv := coverage{w, h, make([]int, (w+1)*(h+1))}
	for y := range h {
		for x := range w {
			cv.sum[(y+1)*(w+1)+x+1] = on[y*w+x] + cv.sum[y*(w+1)+x+1] + cv.sum[(y+1)*(w+1)+x] - cv.sum[y*(w+1)+x]
		}
	}
	return cv
}

// count returns the hero pixels in [x0,x1) x [y0,y1), clipped to the image.
func (cv coverage) count(x0, y0, x1, y1 int) int {
	x0, y0, x1, y1 = max(0, x0), max(0, y0), min(cv.w, x1), min(cv.h, y1)
	if x0 >= x1 || y0 >= y1 {
		return 0
	}
	w := cv.w + 1
	return cv.sum[y1*w+x1] - cv.sum[y0*w+x1] - cv.sum[y1*w+x0] + cv.sum[y0*w+x0]
}

// mix lays s over d at opacity a/256.
func mix(d, s color.RGBA, a int) color.RGBA {
	m := func(d, s uint8) uint8 {
		v := (int(d)*(256-a) + int(s)*a + 128) >> 8
		if v < 0 || v > 255 {
			panic(fmt.Sprintf("opacity %d outside 0-256", a)) // amounts are constants in this file
		}
		return uint8(v)
	}
	return color.RGBA{m(d.R, s.R), m(d.G, s.G), m(d.B, s.B), 0xff}
}

func hexColor(s string) color.RGBA {
	var r, g, b uint8
	if _, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b); err != nil {
		panic(fmt.Sprintf("bad colour %q: %v", s, err)) // colours are constants in this file
	}
	return color.RGBA{r, g, b, 0xff}
}

// Assets returns every generated brand file by name, for go generate and for
// the test that keeps the committed copies current.
func Assets() map[string][]byte {
	a := map[string][]byte{
		"hero-light.svg":     heroSVG(false),
		"hero-dark.svg":      heroSVG(true),
		"proof-light.svg":    proofSVG(false),
		"proof-dark.svg":     proofSVG(true),
		"social-preview.png": socialPNG(),
	}
	for _, dark := range []bool{false, true} {
		theme := "light"
		if dark {
			theme = "dark"
		}
		a["how-bridge-"+theme+".gif"] = sceneGIF(bridgeScene(), dark, bridgeCell, bridgeStep)
		for _, f := range forges {
			a["forge-"+f.slug+"-"+theme+".svg"] = sceneSVG(forgeScene(f), dark, forgeCell, f.name)
		}
	}
	return a
}

// The bridge GIF draws each cell 10 px wide and moves its commits 5 px a
// frame; forge logos draw each cell 4 px wide, 96 px in all.
const (
	bridgeCell, bridgeStep = 10, 5
	forgeCell              = 4
)
