package brand

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Goldens come from prototype/brand/bridges.prototype.html, the approved design.
func TestBridgesMatchPrototype(t *testing.T) {
	raw, err := os.ReadFile("testdata/bridges.golden")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, block := range strings.Split(strings.TrimSpace(string(raw)), "== ")[1:] {
		name, art, _ := strings.Cut(block, "\n")
		want[name] = strings.TrimRight(art, "\n")
	}
	if len(want) != len(Bridges) {
		t.Fatalf("golden has %d bridges, code has %d", len(want), len(Bridges))
	}
	var lv [H][W]int
	for _, b := range Bridges {
		got := strings.Join(rows(b, lv, Plain), "\n")
		if got != want[b.Name] {
			t.Errorf("%s differs from prototype\ngot:\n%s\nwant:\n%s", b.Name, got, want[b.Name])
		}
	}
}

func TestBusyMatchesPrototype(t *testing.T) {
	raw, err := os.ReadFile("testdata/busy-20261016.golden")
	if err != nil {
		t.Fatal(err)
	}
	lv := busy(20261016)
	var sb strings.Builder
	for y := range H {
		for x := range W {
			if lv[y][x] < 1 {
				t.Fatalf("level 0 at %d,%d: every pixel must be green", x, y)
			}
			sb.WriteByte(byte('0' + lv[y][x]))
		}
		sb.WriteByte('\n')
	}
	if sb.String() != string(raw) {
		t.Errorf("busy greens differ from prototype\ngot:\n%swant:\n%s", sb.String(), raw)
	}
}

func TestPickRotatesDaily(t *testing.T) {
	day := time.Date(2026, 10, 16, 23, 59, 0, 0, time.Local)
	b1, s1 := Pick(day)
	b2, _ := Pick(time.Date(2026, 10, 16, 0, 1, 0, 0, time.Local))
	if b1.Name != b2.Name {
		t.Errorf("same day picked %s and %s", b1.Name, b2.Name)
	}
	if s1 != 20261016 {
		t.Errorf("seed = %d, want 20261016", s1)
	}
	seen := map[string]bool{}
	for i := range Bridges {
		b, _ := Pick(day.AddDate(0, 0, i))
		seen[b.Name] = true
	}
	if len(seen) != len(Bridges) {
		t.Errorf("%d consecutive days showed %d bridges, want all", len(Bridges), len(seen))
	}
}
