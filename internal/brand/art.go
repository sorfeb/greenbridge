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

// gallerySVG shows every bridge in the rotation, two per row, with captions.
func gallerySVG(dark bool) []byte {
	const (
		size, cols    = 6, 2
		colW, colGap  = W * size, 48
		rowH, caption = H*size + 48, H*size + 22
	)
	p := themePalette(dark)
	rows := (len(Bridges) + cols - 1) / cols
	w, h := cols*colW+(cols-1)*colGap, rows*rowH-20
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n", w, h, w, h)
	sb.WriteString("<title>The bridges gb rotates through, one per day</title>\n")
	fmt.Fprintf(&sb, `<g font-family="-apple-system,BlinkMacSystemFont,'Segoe UI','Noto Sans',Helvetica,Arial,sans-serif" font-size="13" fill="%s">`+"\n", p.muted)
	for i, b := range Bridges {
		ox, oy := (i%cols)*(colW+colGap), (i/cols)*rowH
		svgCells(&sb, bridgeCells(b, artSeed+uint32(i), p, 0, 0, 1), size, ox, oy)
		fmt.Fprintf(&sb, `<text x="%d" y="%d"><tspan font-weight="600" fill="%s">%s</tspan>, %s</text>`+"\n",
			ox, oy+caption, p.fg, xmlEscape.Replace(b.Name), xmlEscape.Replace(b.Where))
	}
	sb.WriteString("</g>\n</svg>\n")
	return []byte(sb.String())
}

// socialPNG is GitHub's 1280x640 social preview: the dark hero on GitHub's
// dark canvas.
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
	bg := hexColor("#0d1117")
	for y := range h {
		for x := range w {
			img.Set(x, y, bg)
		}
	}
	ox, oy := (w-heroW*size)/2, (h-heroH*size)/2
	for _, c := range hero(themePalette(true)) {
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
	return map[string][]byte{
		"hero-light.svg":     heroSVG(false),
		"hero-dark.svg":      heroSVG(true),
		"bridges-light.svg":  gallerySVG(false),
		"bridges-dark.svg":   gallerySVG(true),
		"social-preview.png": socialPNG(),
	}
}
