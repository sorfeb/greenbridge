package brand

import (
	"strings"
	"testing"
)

// The sample shows a clear gain: mirrored work turns a sparse graph into a busy one.
func TestProofShowsAGain(t *testing.T) {
	personal, work := proofCounts()
	var before, after int
	for y := range proofDays {
		for x := range proofWeeks {
			if personal[y][x] > 0 {
				before++
			}
			if personal[y][x]+work[y][x] > 0 {
				after++
			}
		}
	}
	if after < 3*before {
		t.Errorf("after has %d active days, before %d: the sample should show a clear gain", after, before)
	}
}

// gbridge mirrors real activity and never pads: the sample must show a stretch
// with no work commits, as a holiday leaves.
func TestProofKeepsAGap(t *testing.T) {
	_, work := proofCounts()
	run, longest := 0, 0
	for x := range proofWeeks {
		for y := 1; y <= 5; y++ { // Monday to Friday
			if work[y][x] == 0 {
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
	}
	if longest < 5 {
		t.Errorf("longest weekday run without work commits is %d, want a week or more", longest)
	}
}

func TestProofSVGThemes(t *testing.T) {
	light, dark := string(proofSVG(false)), string(proofSVG(true))
	if light == dark {
		t.Error("light and dark proof graphs are identical")
	}
	for _, s := range []string{light, dark} {
		for _, want := range []string{"Before", "After", "sample data"} {
			if !strings.Contains(s, want) {
				t.Errorf("proof graph lacks %q", want)
			}
		}
	}
}
