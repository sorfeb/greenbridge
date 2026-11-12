package brand

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
	"strings"
)

// shades maps scene tokens to colours.
type shades map[string]shade

// shade is a scene colour: a hex value, with alpha for shadows and highlights.
type shade struct {
	hex   string
	alpha float64
}

// sceneShades extends the theme palette with the ramps scenes light with.
func sceneShades(dark bool) shades {
	m := shades{
		"n0": {"#424a53", 1}, "n1": {"#6e7781", 1}, "n2": {"#8c959f", 1}, "n3": {"#afb8c1", 1}, "n4": {"#d0d7de", 1},
		"l0": {"#174d28", 1}, "l1": {"#216e39", 1}, "l2": {"#30a14e", 1}, "l3": {"#40c463", 1}, "l4": {"#9be9a8", 1},
		"p": {"#ffffff", 1}, "p2": {"#e8ecf0", 1}, "sh": {"#1f2328", .16}, "hl": {"#ffffff", .75},
	}
	if dark {
		m = shades{
			"n0": {"#21262d", 1}, "n1": {"#30363d", 1}, "n2": {"#484f58", 1}, "n3": {"#6e7681", 1}, "n4": {"#8b949e", 1},
			"l0": {"#0e4429", 1}, "l1": {"#006d32", 1}, "l2": {"#26a641", 1}, "l3": {"#39d353", 1}, "l4": {"#8ee8a6", 1},
			"p": {"#adbac7", 1}, "p2": {"#8b98a5", 1}, "sh": {"#000000", .45}, "hl": {"#ffffff", .45},
		}
	}
	for i, g := range themePalette(dark).green {
		m[greenTok[i]] = shade{g, 1}
	}
	return m
}

func (sh shades) of(tok string) shade {
	if strings.HasPrefix(tok, "#") {
		return shade{tok, 1}
	}
	c, ok := sh[tok]
	if !ok {
		panic("unknown scene token " + tok) // tokens are constants in this package
	}
	return c
}

// over blends c onto an opaque background.
func (c shade) over(bg color.RGBA) color.RGBA {
	f := hexColor(c.hex)
	mix := func(a, b uint8) uint8 { return uint8(math.Round(float64(a)*c.alpha + float64(b)*(1-c.alpha))) }
	return color.RGBA{mix(f.R, bg.R), mix(f.G, bg.G), mix(f.B, bg.B), 0xff}
}

// sceneGIF animates a scene's flows over GitHub's page background. Each cell
// is cell px wide; frames advance step px, 20 ms apart, and loop seamlessly.
func sceneGIF(s *scene, dark bool, cell, step int) []byte {
	sh := sceneShades(dark)
	bg := hexColor("#ffffff")
	if dark {
		bg = hexColor("#0d1117")
	}
	pal := color.Palette{bg}
	idx := map[string]uint8{}
	at := func(tok string) uint8 {
		if i, ok := idx[tok]; ok {
			return i
		}
		i := len(pal)
		if i > math.MaxUint8 {
			panic("scene uses more than 256 colours") // a GIF palette holds 256
		}
		pal = append(pal, sh.of(tok).over(bg))
		idx[tok] = uint8(i)
		return idx[tok]
	}
	keys := s.sortedCells()
	for _, k := range keys {
		at(s.cells[k])
	}
	for _, b := range s.blocks {
		at(b.tok)
	}
	period := 0
	for _, f := range s.flows {
		for _, t := range f.toks {
			at(t)
		}
		period = max(period, f.gap*cell)
	}
	gap := cell / 5
	square := func(img *image.Paletted, px, py, size int, ci uint8, clip image.Rectangle) {
		for y := range size {
			for x := range size {
				if size > 4 && (x == 0 || x == size-1) && (y == 0 || y == size-1) {
					continue
				}
				if p := image.Pt(px+x, py+y); p.In(clip) {
					img.SetColorIndex(p.X, p.Y, ci)
				}
			}
		}
	}
	bounds := image.Rect(0, 0, s.w*cell, s.h*cell)
	anim := &gif.GIF{}
	for fr := range max(1, period/step) {
		img := image.NewPaletted(bounds, pal)
		for _, k := range keys {
			square(img, k[0]*cell, k[1]*cell, cell-gap, idx[s.cells[k]], bounds)
		}
		for _, b := range s.blocks {
			square(img, b.x*cell, b.y*cell, b.size*cell-gap, idx[b.tok], bounds)
		}
		for _, f := range s.flows {
			clip := image.Rect(f.x0*cell, f.y*cell, (f.x1+1)*cell, (f.y+f.size)*cell)
			off := fr * step % (f.gap * cell)
			for i, x := 0, f.x0-f.gap; x <= f.x1; i, x = i+1, x+f.gap {
				square(img, x*cell+off, f.y*cell, f.size*cell-gap, idx[f.toks[i%len(f.toks)]], clip)
			}
		}
		anim.Image = append(anim.Image, img)
		anim.Delay = append(anim.Delay, 2)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, anim); err != nil {
		panic(err) // encoding in-memory frames cannot fail
	}
	return buf.Bytes()
}

// sceneSVG draws a still scene with a transparent background.
func sceneSVG(s *scene, dark bool, cell float64, title string) []byte {
	sh := sceneShades(dark)
	w, h := float64(s.w)*cell, float64(s.h)*cell
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g" role="img">`+"\n", w, h, w, h)
	fmt.Fprintf(&sb, "<title>%s</title>\n", xmlEscape.Replace(title))
	side := cell * .85
	for _, k := range s.sortedCells() {
		c := sh.of(s.cells[k])
		op := ""
		if c.alpha < 1 {
			op = fmt.Sprintf(` fill-opacity="%g"`, c.alpha)
		}
		fmt.Fprintf(&sb, `<rect x="%g" y="%g" width="%g" height="%g" rx="1" fill="%s"%s/>`+"\n",
			float64(k[0])*cell, float64(k[1])*cell, side, side, c.hex, op)
	}
	sb.WriteString("</svg>\n")
	return []byte(sb.String())
}
