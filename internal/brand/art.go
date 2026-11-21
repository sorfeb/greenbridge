package brand

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
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

// socialPNG is GitHub's 1280x640 social preview: the dark hero over a faint
// ASCII haze on GitHub's dark canvas.
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
	haze(img, newCoverage(w, h, cells, size, ox, oy))
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

// The haze behind the social hero: faint glyphs from a density ramp over a
// soft wash, densest in a glow under the deck and fading to bare corners,
// under CRT scanlines. Amounts are integers with 256 as 1.0, so every
// platform draws the same pixels (arm64 may fuse float maths into FMA).
const (
	hazeRamp             = " .'-:=+*#%"
	hazeTint             = "#5f9a6e"
	hazeCellW, hazeCellH = 8, 12
	hazeGlyph            = 60 // glyph opacity at full haze: dimmer than the dimmest green even then
	hazeWash             = 20 // wash opacity at full haze
	hazeClear            = 3  // px left bare around each hero square
	hazeSoft             = 22 // radius px over which nearby hero squares fade the glyphs
	scanEvery, scanDark  = 3, 77
)

// asciiGlyphs is a 5x7 pixel face for the haze ramp.
var asciiGlyphs = map[byte][7]string{
	'.':  {".....", ".....", ".....", ".....", ".....", "..#..", "....."},
	'\'': {"..#..", "..#..", ".....", ".....", ".....", ".....", "....."},
	'-':  {".....", ".....", ".....", ".###.", ".....", ".....", "....."},
	':':  {".....", "..#..", ".....", ".....", ".....", "..#..", "....."},
	'=':  {".....", ".....", "#####", ".....", "#####", ".....", "....."},
	'+':  {".....", "..#..", "..#..", "#####", "..#..", "..#..", "....."},
	'*':  {".....", "#.#.#", ".###.", "#####", ".###.", "#.#.#", "....."},
	'#':  {".#.#.", ".#.#.", "#####", ".#.#.", "#####", ".#.#.", ".#.#."},
	'%':  {"##..#", "##..#", "...#.", "..#..", ".#...", "#..##", "#..##"},
}

func haze(img *image.RGBA, hero coverage) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	bg, tint := hexColor(socialBg), hexColor(hazeTint)
	field := hazeField(w, h)
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, mix(bg, tint, hazeWash*field(x, y)/256))
		}
	}
	cols, rows := w/hazeCellW, h/hazeCellH
	padX, padY := (w-cols*hazeCellW)/2, (h-rows*hazeCellH)/2
	jitter := mulberry(artSeed + 2)
	for cy := range rows {
		for cx := range cols {
			x0, y0 := padX+cx*hazeCellW, padY+cy*hazeCellH
			mx, my := x0+hazeCellW/2, y0+hazeCellH/2
			f := field(mx, my)
			// Jitter of up to 0.7 of a ramp step breaks up the contour lines.
			j := int(jitter.next()*358) - 179
			ch := hazeRamp[max(0, min(len(hazeRamp)-1, (f*(len(hazeRamp)-1)+j+128)>>8))]
			if ch == ' ' {
				continue
			}
			const r = hazeSoft
			near := min(256, hero.count(mx-r, my-r, mx+r, my+r)*640/(4*r*r))
			a := hazeGlyph * (64 + 3*f/4) / 256 * (256 - near*115/256) / 256
			gx, gy := x0+(hazeCellW-5)/2, y0+(hazeCellH-7)/2
			for py, row := range asciiGlyphs[ch] {
				for px, on := range row {
					x, y := gx+px, gy+py
					if on != '#' || hero.count(x-hazeClear, y-hazeClear, x+hazeClear+1, y+hazeClear+1) > 0 {
						continue
					}
					img.SetRGBA(x, y, mix(img.RGBAAt(x, y), tint, a))
				}
			}
		}
	}
	black := color.RGBA{A: 0xff}
	for y := 0; y < h; y += scanEvery {
		for x := range w {
			img.SetRGBA(x, y, mix(img.RGBAAt(x, y), black, scanDark))
		}
	}
}

// hazeField returns the haze amount, 0-256, at a pixel: a base level, a glow
// centred under the deck, and noise clouds, faded toward the corners.
func hazeField(w, h int) func(x, y int) int {
	coarse := newValueNoise(w, h, 120, artSeed)
	fine := newValueNoise(w, h, 48, artSeed+1)
	return func(x, y int) int {
		n := (3*coarse.at(x, y) + 2*fine.at(x, y)) / 5
		// The glow falls off as (1-q)^2 over an ellipse, close to a gaussian.
		const rx, ry = 1100, 450
		dx, dy := x-w/2, y-h*6/10
		q := min(256, dx*dx*256/(rx*rx)+dy*dy*256/(ry*ry))
		glow := (256 - q) * (256 - q) / 256
		f := max(0, min(256, 77+glow*115/256+(n-128)*179/256))
		du, dv := (2*x-w)*256/w, (2*y-h)*256/h
		vignette := max(0, 256-(du*du+dv*dv)/512*333/256)
		return f * vignette / 256
	}
}

// valueNoise is smooth noise: random values 0-255 on a lattice scale px
// apart, eased between.
type valueNoise struct {
	scale int
	lat   [][]int
}

func newValueNoise(w, h, scale int, seed uint32) valueNoise {
	r := mulberry(seed)
	lat := make([][]int, h/scale+2)
	for j := range lat {
		lat[j] = make([]int, w/scale+2)
		for i := range lat[j] {
			lat[j][i] = int(r.next() * 256)
		}
	}
	return valueNoise{scale, lat}
}

func (n valueNoise) at(x, y int) int {
	fx, fy := x*256/n.scale, y*256/n.scale
	i, j := fx>>8, fy>>8
	ease := func(t int) int { return t * t * (768 - 2*t) >> 16 }
	sx, sy := ease(fx&255), ease(fy&255)
	top := n.lat[j][i]*(256-sx) + n.lat[j][i+1]*sx
	bottom := n.lat[j+1][i]*(256-sx) + n.lat[j+1][i+1]*sx
	return (top*(256-sy) + bottom*sy) >> 16
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
