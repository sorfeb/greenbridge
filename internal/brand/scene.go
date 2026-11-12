package brand

import (
	"cmp"
	"maps"
	"math"
	"slices"
)

// A scene is a grid of tokens drawn with lit 3D primitives: light comes from
// the upper left, and each primitive picks a shade from a ramp (dark to light)
// by its surface normal. Tokens resolve to colours per theme at render time;
// a token starting with # is a literal colour.
type scene struct {
	w, h   int
	cells  map[[2]int]string
	blocks []block
	flows  []flow
}

// block is a contribution square spanning size cells.
type block struct {
	x, y, size int
	tok        string
}

// flow is a row of squares moving right, gap cells apart, clipped to x0..x1.
type flow struct {
	x0, x1, y, size, gap int
	toks                 []string
}

type ramp []string

var (
	neutral = ramp{"n0", "n1", "n2", "n3", "n4"}
	leaf    = ramp{"l0", "l1", "l2", "l3", "l4"}
)

func newScene(w, h int) *scene { return &scene{w: w, h: h, cells: map[[2]int]string{}} }

func lit(nx, ny, nz float64) float64 { return -.5*nx - .6*ny + .62*nz }

func (r ramp) pick(l float64) string {
	return r[min(max(int(math.Floor((l+.25)/1.25*float64(len(r)))), 0), len(r)-1)]
}

func (s *scene) set(x, y float64, tok string) {
	ix, iy := round(x), round(y)
	if ix >= 0 && iy >= 0 && ix < s.w && iy < s.h {
		s.cells[[2]int{ix, iy}] = tok
	}
}

// fill calls fn for every cell in the box; an empty token leaves the cell.
func (s *scene) fill(x0, y0, x1, y1 float64, fn func(x, y float64) string) {
	for y := math.Floor(y0); y <= math.Ceil(y1); y++ {
		for x := math.Floor(x0); x <= math.Ceil(x1); x++ {
			if t := fn(x, y); t != "" {
				s.set(x, y, t)
			}
		}
	}
}

func (s *scene) rect(x0, y0, x1, y1 float64, tok string) {
	s.fill(x0, y0, x1, y1, func(_, _ float64) string { return tok })
}

func (s *scene) disc(cx, cy, rx, ry float64, r ramp) {
	s.fill(cx-rx, cy-ry, cx+rx, cy+ry, func(x, y float64) string {
		nx, ny := (x-cx)/rx, (y-cy)/ry
		d := nx*nx + ny*ny
		if d > 1 {
			return ""
		}
		return r.pick(lit(nx, ny, math.Sqrt(1-d)))
	})
}

// ring is a torus seen from the front, inner radius r0, outer r1.
func (s *scene) ring(cx, cy, r0, r1 float64, r ramp) {
	rm, hw := (r0+r1)/2, (r1-r0)/2
	s.fill(cx-r1, cy-r1, cx+r1, cy+r1, func(x, y float64) string {
		dx, dy := x-cx, y-cy
		d := math.Hypot(dx, dy)
		if d < r0-.4 || d > r1+.4 {
			return ""
		}
		t := min(max((d-rm)/hw, -1), 1)
		if d == 0 {
			d = 1
		}
		return r.pick(lit(dx/d*t, dy/d*t, math.Sqrt(max(0, 1-t*t))))
	})
}

func (s *scene) cylH(x0, x1, yc, rad float64, r ramp) {
	s.fill(x0, yc-rad, x1, yc+rad, func(_, y float64) string {
		ny := (y - yc) / rad
		if math.Abs(ny) > 1 {
			return ""
		}
		return r.pick(lit(0, ny, math.Sqrt(1-ny*ny)))
	})
}

func (s *scene) cylV(xc, rad, y0, y1 float64, r ramp) {
	s.fill(xc-rad, y0, xc+rad, y1, func(x, _ float64) string {
		nx := (x - xc) / rad
		if math.Abs(nx) > 1 {
			return ""
		}
		return r.pick(lit(nx, 0, math.Sqrt(1-nx*nx)))
	})
}

// capsule is a rod of radius rad from (x0,y0) to (x1,y1).
func (s *scene) capsule(x0, y0, x1, y1, rad float64, r ramp) {
	vx, vy := x1-x0, y1-y0
	l2 := vx*vx + vy*vy
	if l2 == 0 {
		l2 = 1
	}
	s.fill(min(x0, x1)-rad, min(y0, y1)-rad, max(x0, x1)+rad, max(y0, y1)+rad, func(x, y float64) string {
		t := min(max(((x-x0)*vx+(y-y0)*vy)/l2, 0), 1)
		dx, dy := (x-x0-t*vx)/rad, (y-y0-t*vy)/rad
		d := dx*dx + dy*dy
		if d > 1 {
			return ""
		}
		return r.pick(lit(dx, dy, math.Sqrt(1-d)))
	})
}

func (s *scene) poly(pts [][2]float64, fn func(x, y float64) string) {
	x0, y0, x1, y1 := pts[0][0], pts[0][1], pts[0][0], pts[0][1]
	for _, p := range pts {
		x0, y0, x1, y1 = min(x0, p[0]), min(y0, p[1]), max(x1, p[0]), max(y1, p[1])
	}
	s.fill(x0, y0, x1, y1, func(x, y float64) string {
		in := false
		for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
			xi, yi, xj, yj := pts[i][0], pts[i][1], pts[j][0], pts[j][1]
			if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
				in = !in
			}
		}
		if !in {
			return ""
		}
		return fn(x, y)
	})
}

func solid(tok string) func(_, _ float64) string { return func(_, _ float64) string { return tok } }

// shaded lights a flat shape as if it bulged around (cx, cy).
func shaded(r ramp, cx, cy, rad float64) func(x, y float64) string {
	return func(x, y float64) string { return r.pick(lit((x-cx)/rad, (y-cy)/rad, .55)) }
}

func (s *scene) line(x0, y0, x1, y1 float64, tok string) {
	n := max(math.Abs(x1-x0), math.Abs(y1-y0))
	if n == 0 {
		n = 1
	}
	for i := 0.0; i <= n; i++ {
		s.set(x0+(x1-x0)*i/n, y0+(y1-y0)*i/n, tok)
	}
}

func (s *scene) shadow(cx, cy, rx, ry float64) { s.disc(cx, cy, rx, ry, ramp{"sh"}) }

// graph adds contribution squares with levels lo..hi from a fixed seed.
func (s *scene) graph(x, y, cols, rows, size int, seed uint32, lo, hi int) {
	r := lcg(seed)
	for i := range cols {
		for j := range rows {
			lv := lo + int(r()*float64(hi-lo+1))
			s.blocks = append(s.blocks, block{x + i*size, y + j*size, size, greenTok[lv]})
		}
	}
}

var greenTok = [5]string{"g0", "g1", "g2", "g3", "g4"}

// lcg is the prototype's generator, so seeds draw the same squares.
func lcg(seed uint32) func() float64 {
	s := seed
	return func() float64 {
		s = s*1664525 + 1013904223
		return float64(s) / (1 << 32)
	}
}

// sortedCells returns cells in row order, for stable output.
func (s *scene) sortedCells() [][2]int {
	return slices.SortedFunc(maps.Keys(s.cells), func(a, b [2]int) int {
		return cmp.Or(cmp.Compare(a[1], b[1]), cmp.Compare(a[0], b[0]))
	})
}
