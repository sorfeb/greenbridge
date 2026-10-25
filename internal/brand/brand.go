// Package brand draws the gbridge banner: a famous bridge in square pixels,
// coloured like a busy contribution graph. The bridge and its greens change
// once a day and are the same for every run and every user on that day.
package brand

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Pick returns the bridge of the day and the seed for its greens. Both come
// from the local calendar date, so no state is kept.
func Pick(day time.Time) (Bridge, uint32) {
	y, m, d := day.Date()
	days := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
	return Bridges[days%int64(len(Bridges))], uint32(y*10000 + int(m)*100 + d) //nolint:gosec // YYYYMMDD fits uint32 until year 429496
}

// mulberry32, matching the prototype so a seed draws the same greens.
type mulberry uint32

func (m *mulberry) next() float64 {
	*m += 0x6D2B79F5
	a := uint32(*m)
	t := (a ^ a>>15) * (1 | a)
	t = (t + (t^t>>7)*(61|t)) ^ t
	return float64(t^t>>14) / 4294967296
}

// busy returns a green level 1-4 per pixel. Columns are weeks with their own
// intensity and weekend rows run lighter. Never level 0: every pixel is green.
func busy(seed uint32) [H][W]int {
	r := mulberry(seed)
	var week [W]float64
	for x := range week {
		switch p := r.next(); {
		case p < .3:
			week[x] = .7
		case p < .75:
			week[x] = .85
		default:
			week[x] = 1
		}
	}
	var lv [H][W]int
	for y := range H {
		weekend := y%7 == 0 || y%7 == 6
		for x := range W {
			v := week[x] * (.5 + .7*r.next())
			if weekend {
				v -= .2
			}
			switch {
			case v < .45:
				lv[y][x] = 1
			case v < .65:
				lv[y][x] = 2
			case v < .85:
				lv[y][x] = 3
			default:
				lv[y][x] = 4
			}
		}
	}
	return lv
}

// Greens are GitHub's contribution colours, index 0 the empty day.
var Greens = [5]lipgloss.AdaptiveColor{
	{Light: "#d0d7de", Dark: "#2a332e"},
	{Light: "#9be9a8", Dark: "#0e4429"},
	{Light: "#40c463", Dark: "#006d32"},
	{Light: "#30a14e", Dark: "#26a641"},
	{Light: "#216e39", Dark: "#39d353"},
}

// Painter colours a string at a contribution level. Plain returns it as is.
type Painter func(level int, s string) string

// Plain is the Painter for output without colour.
func Plain(_ int, s string) string { return s }

// Paint returns a Painter for the renderer's terminal. It degrades to plain
// text when the renderer has no colour (pipes, NO_COLOR).
func Paint(r *lipgloss.Renderer) Painter {
	var styles [5]lipgloss.Style
	for i, c := range Greens {
		styles[i] = r.NewStyle().Foreground(c)
	}
	return func(level int, s string) string { return styles[level].Render(s) }
}

// rows renders a bridge in half blocks, each cell coloured by its top pixel.
func rows(b Bridge, lv [H][W]int, paint Painter) []string {
	var g grid
	b.draw(&g)
	out := make([]string, 0, H/2)
	for y := 0; y < H; y += 2 {
		var sb strings.Builder
		for x := range W {
			top, bot := g[y][x], g[y+1][x] //nolint:gosec // G602 false positive: y steps by 2 over an even H
			switch {
			case top && bot:
				sb.WriteString(paint(lv[y][x], "█"))
			case top:
				sb.WriteString(paint(lv[y][x], "▀"))
			case bot:
				sb.WriteString(paint(lv[y][x], "▄"))
			default:
				sb.WriteByte(' ')
			}
		}
		out = append(out, strings.TrimRight(sb.String(), " "))
	}
	return out
}

// Banner writes the bridge of the day, the spaced name, and the bridge's caption.
func Banner(w io.Writer, day time.Time, r *lipgloss.Renderer) {
	b, seed := Pick(day)
	for _, row := range rows(b, busy(seed), Paint(r)) {
		fmt.Fprintln(w, row)
	}
	name := "g r e e n b r i d g e"
	fmt.Fprintf(w, "\n%s%s\n", strings.Repeat(" ", (W-len(name))/2), r.NewStyle().Bold(true).Render(name))
	fmt.Fprintln(w, r.NewStyle().Faint(true).Render(b.Name+", "+b.Where))
	fmt.Fprintln(w)
}
