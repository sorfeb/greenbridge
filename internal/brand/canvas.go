package brand

import "math"

// W and H are the pixel size of every bridge. Half blocks render two pixel
// rows per text row, so a banner is W columns by H/2 rows.
const (
	W = 40
	H = 14
)

type grid [H][W]bool

// round matches JavaScript's Math.round, which the prototype was drawn with.
func round(f float64) int { return int(math.Floor(f + 0.5)) }

func (g *grid) put(x, y int, v bool) {
	if x >= 0 && x < W && y >= 0 && y < H {
		g[y][x] = v
	}
}

func (g *grid) set(x, y int)   { g.put(x, y, true) }
func (g *grid) unset(x, y int) { g.put(x, y, false) }

func (g *grid) fill(x0, y0, x1, y1 int, v bool) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			g.put(x, y, v)
		}
	}
}

func (g *grid) rect(x0, y0, x1, y1 int)  { g.fill(x0, y0, x1, y1, true) }
func (g *grid) clear(x0, y0, x1, y1 int) { g.fill(x0, y0, x1, y1, false) }

// line draws a Bresenham line.
func (g *grid) line(x0, y0, x1, y1 int) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := step(x0, x1), step(y0, y1)
	e := dx + dy
	for {
		g.set(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

// cable draws from (xa,ya) to (xb,yb) with a parabolic sag: positive sags
// down, negative arches up. It returns the y of each column for hangers.
func (g *grid) cable(xa, ya, xb, yb int, sag float64) map[int]int {
	ys := map[int]int{}
	prev, started := 0, false
	for x := xa; x <= xb; x++ {
		t := float64(x-xa) / float64(xb-xa)
		y := round(float64(ya) + float64(yb-ya)*t + sag*4*t*(1-t))
		ys[x] = y
		if started {
			g.rect(x, min(prev, y), x, max(prev, y))
		}
		g.set(x, y)
		prev, started = y, true
	}
	return ys
}

// hangers drops verticals from a cable to the deck every step columns.
func (g *grid) hangers(ys map[int]int, x0, x1, step, deck int) {
	for x := x0; x <= x1; x += step {
		if y, ok := ys[x]; ok {
			g.rect(x, min(y, deck), x, max(y, deck))
		}
	}
}

// opening clears a round-topped opening.
func (g *grid) opening(x0, x1, yTop, yBase int) {
	a := float64(x1-x0+1) / 2
	cx := float64(x0) + a
	r := math.Min(a, float64(yBase-yTop+1))
	spring := float64(yTop) + r
	for y := yTop; y <= yBase; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+.5, float64(y)+.5
			if py >= spring || sq((px-cx)/a)+sq((py-spring)/r) <= 1 {
				g.unset(x, y)
			}
		}
	}
}

// arcade fills a block and cuts n openings separated by piers.
func (g *grid) arcade(x0, x1, y0, y1, n, pier, rise int) {
	g.rect(x0, y0, x1, y1)
	w := float64(x1-x0+1-pier) / float64(n)
	for i := 0; i < n; i++ {
		g.opening(round(float64(x0+pier)+float64(i)*w), round(float64(x0)+float64(i+1)*w)-1, y0+rise, y1)
	}
}

// mass fills each column from a top profile down to the bottom edge.
func (g *grid) mass(x0, x1 int, top func(x float64) float64) {
	for x := x0; x <= x1; x++ {
		g.rect(x, round(top(float64(x))), x, H-1)
	}
}

func sq(f float64) float64 { return f * f }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func step(from, to int) int {
	if from < to {
		return 1
	}
	return -1
}
