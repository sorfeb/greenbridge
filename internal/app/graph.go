package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/brand"
	"github.com/sorfeb/greenbridge/internal/mirror"
)

var shades = []string{"·", "░", "▒", "▓", "█"}

func level(n int) int {
	switch {
	case n == 0:
		return 0
	case n <= 2:
		return 1
	case n <= 5:
		return 2
	case n <= 9:
		return 3
	default:
		return 4
	}
}

// DayKey buckets an entry by the calendar date in its own UTC offset.
func DayKey(t time.Time) string {
	return t.Format("2006-01-02")
}

// Graph renders a contribution-style grid of the last weeks, ending today.
// Rows are weekdays (Sunday first), columns are weeks. paint colours each cell.
func Graph(w io.Writer, entries []mirror.Entry, weeks int, today time.Time, paint brand.Painter) {
	counts := map[string]int{}
	for _, e := range entries {
		counts[DayKey(e.Time)]++
	}
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	end := today.AddDate(0, 0, 6-int(today.Weekday())) // Saturday of this week
	start := end.AddDate(0, 0, -7*weeks+1)             // a Sunday
	labels := []string{"Sun", "   ", "Tue", "   ", "Thu", "   ", "Sat"}
	months := make([]byte, weeks)
	for i := range months {
		months[i] = ' '
	}
	for c := range weeks {
		d := start.AddDate(0, 0, 7*c)
		if d.Day() <= 7 {
			months[c] = d.Format("Jan")[0]
		}
	}
	fmt.Fprintf(w, "    %s\n", string(months))
	for r := range 7 {
		var b strings.Builder
		for c := range weeks {
			d := start.AddDate(0, 0, 7*c+r)
			if d.After(today) {
				b.WriteString(" ")
				continue
			}
			l := level(counts[d.Format("2006-01-02")])
			b.WriteString(paint(l, shades[l]))
		}
		fmt.Fprintf(w, "%s %s\n", labels[r], b.String())
	}
	var legend strings.Builder
	for l, s := range shades {
		legend.WriteString(paint(l, s))
	}
	fmt.Fprintf(w, "    less %s more\n", legend.String())
}

// YearTable prints commit and active-day counts per year.
func YearTable(w io.Writer, entries []mirror.Entry) {
	commits := map[int]int{}
	days := map[int]map[string]bool{}
	for _, e := range entries {
		y := e.Time.Year()
		commits[y]++
		if days[y] == nil {
			days[y] = map[string]bool{}
		}
		days[y][DayKey(e.Time)] = true
	}
	var years []int
	for y := range commits {
		years = append(years, y)
	}
	sort.Ints(years)
	fmt.Fprintf(w, "%-6s %8s %12s\n", "year", "commits", "active days")
	for _, y := range years {
		fmt.Fprintf(w, "%-6d %8d %12d\n", y, commits[y], len(days[y]))
	}
}
