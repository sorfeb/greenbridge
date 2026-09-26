package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/mirror"
)

var shades = []string{"·", "░", "▒", "▓", "█"}

func shade(n int) string {
	switch {
	case n == 0:
		return shades[0]
	case n <= 2:
		return shades[1]
	case n <= 5:
		return shades[2]
	case n <= 9:
		return shades[3]
	default:
		return shades[4]
	}
}

// DayKey buckets an entry by the calendar date in its own UTC offset.
func DayKey(t time.Time) string {
	return t.Format("2006-01-02")
}

// Graph renders a contribution-style grid of the last weeks, ending today.
// Rows are weekdays (Sunday first), columns are weeks.
func Graph(w io.Writer, entries []mirror.Entry, weeks int, today time.Time) {
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
	for c := 0; c < weeks; c++ {
		d := start.AddDate(0, 0, 7*c)
		if d.Day() <= 7 {
			months[c] = d.Format("Jan")[0]
		}
	}
	fmt.Fprintf(w, "    %s\n", string(months))
	for r := 0; r < 7; r++ {
		var b strings.Builder
		for c := 0; c < weeks; c++ {
			d := start.AddDate(0, 0, 7*c+r)
			if d.After(today) {
				b.WriteString(" ")
				continue
			}
			b.WriteString(shade(counts[d.Format("2006-01-02")]))
		}
		fmt.Fprintf(w, "%s %s\n", labels[r], b.String())
	}
	fmt.Fprintf(w, "    less %s more\n", strings.Join(shades, ""))
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
