package brand

import (
	"fmt"
	"slices"
	"strings"
)

// The proof graph is a year of contribution squares before and after a sync.
// Its data is generated, not taken from a profile, and the picture says so.
const (
	proofWeeks = 53
	proofDays  = 7 // rows, Sunday first
	proofSeed  = 20261026
)

// proofCounts returns commits per day, indexed [weekday][week]: personal work
// that GitHub already sees, and work that gbridge mirrors. Work lands on
// weekdays and stops for a two-week holiday, because gbridge never pads.
func proofCounts() (personal, work [proofDays][proofWeeks]int) {
	r := mulberry(proofSeed)
	for x := range proofWeeks {
		intensity := .4 + .6*r.next()
		holiday := x == 31 || x == 32
		for y := range proofDays {
			weekday := y >= 1 && y <= 5
			pPersonal := .3
			if weekday {
				pPersonal = .08
			}
			if r.next() < pPersonal {
				personal[y][x] = 1 + int(r.next()*3)
			}
			if weekday && !holiday && r.next() < .85 {
				work[y][x] = 1 + int(r.next()*r.next()*12*intensity)
			}
		}
	}
	return personal, work
}

// proofLevels maps counts to contribution levels the way GitHub does: relative
// to the busiest day, in four steps.
func proofLevels(counts [proofDays][proofWeeks]int) [proofDays][proofWeeks]int {
	most := 0
	for y := range proofDays {
		most = max(most, slices.Max(counts[y][:]))
	}
	var lv [proofDays][proofWeeks]int
	for y := range proofDays {
		for x := range proofWeeks {
			if n := counts[y][x]; n > 0 {
				lv[y][x] = (4*n + most - 1) / most
			}
		}
	}
	return lv
}

func proofCells(lv [proofDays][proofWeeks]int, p palette, oy int) []cell {
	var out []cell
	for y := range proofDays {
		for x := range proofWeeks {
			out = append(out, cell{x, oy + y, 1, p.green[lv[y][x]]})
		}
	}
	return out
}

// proofSVG draws the before graph over the after graph.
func proofSVG(dark bool) []byte {
	const size, caption, gap, foot = 10, 22, 30, 38
	p := themePalette(dark)
	personal, work := proofCounts()
	var all [proofDays][proofWeeks]int
	mirrored := 0
	for y := range proofDays {
		for x := range proofWeeks {
			all[y][x] = personal[y][x] + work[y][x]
			mirrored += work[y][x]
		}
	}
	panel := caption + proofDays*size
	w, h := proofWeeks*size, 2*panel+gap+foot
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`+"\n", w, h, w, h)
	sb.WriteString("<title>A contribution graph before and after gbridge sync, drawn from sample data</title>\n")
	fmt.Fprintf(&sb, `<g font-family="-apple-system,BlinkMacSystemFont,'Segoe UI','Noto Sans',Helvetica,Arial,sans-serif" font-size="13" fill="%s">`+"\n", p.muted)
	fmt.Fprintf(&sb, `<text x="0" y="14"><tspan font-weight="600" fill="%s">Before</tspan>, GitHub activity only</text>`+"\n", p.fg)
	svgCells(&sb, proofCells(proofLevels(personal), p, 0), size, 0, caption)
	oy := panel + gap
	fmt.Fprintf(&sb, `<text x="0" y="%d"><tspan font-weight="600" fill="%s">After</tspan>, plus %d work commits mirrored by gbridge sync</text>`+"\n", oy+14, p.fg, mirrored)
	svgCells(&sb, proofCells(proofLevels(all), p, 0), size, 0, oy+caption)
	fmt.Fprintf(&sb, `<text x="0" y="%d">Illustrative sample data, not a real profile.</text>`+"\n", h-18)
	fmt.Fprintf(&sb, `<text x="0" y="%d">The two empty weeks are a holiday: gbridge never fills gaps.</text>`+"\n", h-2)
	sb.WriteString("</g>\n</svg>\n")
	return []byte(sb.String())
}
