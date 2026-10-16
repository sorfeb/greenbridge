package brand

// Bridge is one banner silhouette.
type Bridge struct {
	Name  string
	Where string
	draw  func(g *grid)
}

// Bridges rotate one per day. Drawn on a W x H grid; the prototype on the
// prototype/brand-ascii-logo branch renders the same shapes.
var Bridges = []Bridge{
	{"Golden Gate", "San Francisco, 1937", func(g *grid) {
		const deck = 10
		g.rect(0, deck, 39, deck)
		for _, t := range []int{8, 29} {
			g.rect(t, 0, t, 13)
			g.rect(t+2, 0, t+2, 13)
			for _, b := range []int{0, 3, 6} {
				g.rect(t, b, t+2, b)
			}
		}
		g.hangers(g.cable(10, 0, 29, 0, 9), 12, 27, 3, deck)
		g.hangers(g.cable(0, 9, 8, 0, 2), 2, 7, 2, deck)
		g.hangers(g.cable(31, 0, 39, 9, 2), 32, 38, 2, deck)
	}},
	{"Brooklyn", "New York, 1883", func(g *grid) {
		const deck = 10
		for _, t := range []int{7, 26} {
			g.rect(t, 1, t+6, 13)
			g.rect(t+1, 0, t+5, 0)
			g.opening(t+1, t+2, 3, deck)
			g.opening(t+4, t+5, 3, deck)
		}
		g.rect(0, deck, 39, deck)
		g.hangers(g.cable(13, 1, 26, 1, 8), 15, 24, 3, deck)
		g.hangers(g.cable(0, 9, 7, 1, 2), 2, 6, 2, deck)
		g.hangers(g.cable(32, 1, 39, 9, 2), 33, 38, 2, deck)
		g.line(13, 2, 18, deck)
		g.line(26, 2, 21, deck)
	}},
	{"Tower Bridge", "London, 1894", func(g *grid) {
		const deck = 10
		for _, t := range []int{10, 24} {
			g.rect(t, 3, t+5, 13)
			g.rect(t+1, 1, t+4, 2)
			g.rect(t+2, 0, t+3, 0)
			g.set(t, 1)
			g.set(t+5, 1)
			g.set(t, 2)
			g.set(t+5, 2)
			g.opening(t+2, t+3, 5, 6)
			g.opening(t+1, t+4, 8, 9)
		}
		g.rect(16, 3, 23, 3)
		g.rect(16, 5, 23, 5)
		for x := 17; x <= 23; x += 2 {
			g.set(x, 4)
		}
		g.rect(0, deck, 39, deck)
		g.hangers(g.cable(0, 9, 10, 4, 2), 2, 9, 2, deck)
		g.hangers(g.cable(29, 4, 39, 9, 2), 30, 37, 2, deck)
	}},
	{"Sydney Harbour", "Sydney, 1932", func(g *grid) {
		const deck = 8
		top, bot := g.cable(6, 12, 33, 12, -12), g.cable(8, 12, 31, 12, -9)
		for x := 8; x <= 31; x += 2 {
			yt, ok1 := top[x]
			yb, ok2 := bot[x]
			if ok1 && ok2 {
				g.rect(x, yt, x, yb)
			}
		}
		g.hangers(bot, 12, 27, 3, deck)
		g.rect(0, deck, 39, deck)
		for _, p := range []int{1, 34} {
			g.rect(p, 3, p+4, 9)
			g.rect(p+1, 2, p+3, 2)
		}
		g.rect(5, 12, 8, 13)
		g.rect(31, 12, 34, 13)
	}},
	{"Pont du Gard", "Nîmes, 1st century", func(g *grid) {
		g.arcade(0, 39, 0, 1, 13, 1, 1)
		g.arcade(0, 39, 2, 6, 5, 2, 1)
		g.arcade(3, 36, 7, 13, 4, 3, 1)
	}},
	{"Ponte Vecchio", "Florence, 1345", func(g *grid) {
		g.arcade(0, 39, 9, 13, 3, 3, 1)
		g.rect(0, 8, 39, 8)
		for _, s := range [][3]int{{1, 5, 5}, {6, 9, 4}, {10, 14, 6}, {15, 24, 2}, {25, 29, 5}, {30, 33, 4}, {34, 38, 6}} {
			g.rect(s[0], s[2], s[1], 8)
		}
		g.rect(15, 1, 24, 1)
		for _, x := range []int{17, 19, 21, 23} {
			g.clear(x, 3, x, 4)
		}
		for _, x := range []int{3, 8, 12, 27, 31, 36} {
			g.unset(x, 7)
		}
	}},
	{"Rialto", "Venice, 1591", func(g *grid) {
		g.mass(3, 36, func(x float64) float64 { return 5 + 3*sq((x-19.5)/16.5) })
		g.opening(6, 33, 9, 13)
		for _, x := range []int{8, 11, 14, 25, 28, 31} {
			g.unset(x, 7)
		}
		g.rect(16, 2, 23, 5)
		g.rect(18, 1, 21, 1)
		g.rect(19, 0, 20, 0)
		g.unset(18, 4)
		g.unset(21, 4)
		g.unset(18, 3)
		g.unset(21, 3)
	}},
	{"Forth Bridge", "Edinburgh, 1890", func(g *grid) {
		const deck = 7
		g.rect(0, deck, 39, deck)
		for _, c := range []int{7, 20, 33} {
			g.line(c-6, deck, c-1, 1)
			g.line(c+1, 1, c+6, deck)
			g.rect(c-1, 1, c+1, 1)
			g.line(c-6, deck, c-1, 12)
			g.line(c+6, deck, c+1, 12)
			g.rect(c-1, 1, c-1, 12)
			g.rect(c+1, 1, c+1, 12)
			g.rect(c-2, 12, c+2, 13)
		}
	}},
	{"Millau Viaduct", "Millau, 2004", func(g *grid) {
		const deck = 6
		g.rect(0, deck, 39, deck)
		for _, x := range []int{4, 11, 18, 25, 32} {
			g.rect(x, 0, x, deck)
			g.line(x, 1, x-3, deck)
			g.line(x, 1, x+3, deck)
			g.rect(x-1, deck+1, x+1, deck+2)
			g.rect(x, deck+3, x, 13)
		}
		g.rect(38, deck+1, 39, 13)
		g.rect(0, deck+1, 0, 13)
	}},
	{"Charles Bridge", "Prague, 1402", func(g *grid) {
		g.rect(1, 4, 7, 13)
		g.rect(2, 2, 6, 3)
		g.rect(3, 1, 5, 1)
		g.rect(4, 0, 4, 0)
		g.set(1, 3)
		g.set(7, 3)
		g.opening(3, 5, 9, 13)
		g.unset(4, 6)
		g.rect(8, 7, 39, 7)
		g.arcade(8, 39, 8, 13, 5, 2, 0)
		for x := 10; x <= 38; x += 4 {
			g.rect(x, 5, x, 6)
		}
	}},
}
