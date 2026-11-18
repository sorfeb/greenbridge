package app

import (
	"fmt"
	"io"
	"time"

	"github.com/sorfeb/greenbridge/internal/brand"
)

// Report writes the plan for a person to read: one line per repo, totals,
// the mirror's state, a year table, and a graph of weeks ending today.
func (p *Plan) Report(w io.Writer, today time.Time, weeks int, paint brand.Painter) {
	for _, r := range p.Repos {
		if r.Skipped != "" {
			fmt.Fprintf(w, "  skip %s: %s\n", r.Path, r.Skipped)
		} else {
			fmt.Fprintf(w, "  %5d  %s\n", r.Matched, r.Path)
		}
	}
	WriteFetchFailed(w, p.FetchFailed())
	WriteNewRepos(w, p.NewRepos())
	fmt.Fprintf(w, "\n%d commits matched, %d after dedup", p.Matched, len(p.Entries))
	if len(p.Entries) > 0 {
		fmt.Fprintf(w, ", %s to %s", DayKey(p.Entries[0].Time), DayKey(p.Entries[len(p.Entries)-1].Time))
	}
	fmt.Fprintln(w)
	if p.FetchErr != nil {
		fmt.Fprintf(w, "mirror fetch failed (%v); counts are from the last fetch\n", p.FetchErr)
	}
	fmt.Fprintf(w, "already in mirror: %d, would add: %d\n", p.Ledger, len(p.New))
	WriteOrphans(w, p.Orphans)
	fmt.Fprintln(w)
	YearTable(w, p.Entries)
	fmt.Fprintln(w)
	Graph(w, p.Entries, weeks, today, paint)
}
