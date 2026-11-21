package brand

import (
	"bytes"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The committed assets must be what the code draws today. On failure, run
// go tool -modfile tools.mod task art.
func TestArtUpToDate(t *testing.T) {
	for name, want := range Assets() {
		got, err := os.ReadFile(filepath.Join("..", "..", ".github", "assets", name))
		if err != nil {
			t.Fatalf("%s: %v (run task art)", name, err)
		}
		if filepath.Ext(name) == ".png" {
			// Compare pixels, not bytes: PNG compression may differ between Go releases.
			if !bytes.Equal(pixels(t, got), pixels(t, want)) {
				t.Errorf("%s is stale; run task art", name)
			}
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run task art", name)
		}
	}
}

func pixels(t *testing.T, b []byte) []byte {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba.Pix
}

// The bridge GIF must move, and its last frame must lead back into the first
// without a jump: one loop advances the commits by exactly one gap.
func TestBridgeGIFLoops(t *testing.T) {
	s := bridgeScene()
	g, err := gif.DecodeAll(bytes.NewReader(sceneGIF(s, false, bridgeCell, bridgeStep)))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) < 2 || bytes.Equal(g.Image[0].Pix, g.Image[1].Pix) {
		t.Fatalf("%d frames, first two equal: no motion", len(g.Image))
	}
	if loop := len(g.Image) * bridgeStep; loop != s.flows[0].gap*bridgeCell {
		t.Errorf("loop moves %d px, gap is %d px", loop, s.flows[0].gap*bridgeCell)
	}
}

// Forge logos keep their host's colours and contain no brand green.
func TestForgeLogosKeepHostColours(t *testing.T) {
	host := map[string]string{
		"gitlab": gitlabRamp[2], "bitbucket": bitbucketRamp[2], "gitea": giteaRamp[2],
		"forgejo": forgejoOrange[2], "codeberg": codebergRamp[2], "git": gitRamp[2],
	}
	for slug, c := range host {
		svg := string(Assets()["forge-"+slug+"-light.svg"])
		if !strings.Contains(svg, c) {
			t.Errorf("%s logo lacks its colour %s", slug, c)
		}
		greens := themePalette(false).green
		for _, g := range greens[1:] {
			if strings.Contains(svg, g) {
				t.Errorf("%s logo uses brand green %s", slug, g)
			}
		}
	}
}

func TestWordmarkFitsHero(t *testing.T) {
	for _, c := range hero(themePalette(false)) {
		if c.x < 0 || c.x >= heroW || c.y < 0 || c.y >= heroH {
			t.Fatalf("cell %d,%d outside the %dx%d hero", c.x, c.y, heroW, heroH)
		}
	}
	for _, r := range "greenbridge" {
		if _, ok := glyphs[r]; !ok {
			t.Errorf("no glyph for %q", r)
		}
	}
}

// The social preview's ledger stays behind the hero: the bridge and wordmark
// keep their exact colours, a dark halo leaves the background bare near every
// hero square, and past the halo the ledger reaches every part of the canvas.
func TestSocialLedgerStaysBehindHero(t *testing.T) {
	const w, h, size = 1280, 640, 12
	img := socialImage(w, h, size)
	p := themePalette(true)
	ox, oy := (w-heroW*size)/2, (h-heroH*size)/2
	onHero := make([]bool, w*h)
	for _, c := range hero(p) {
		sq := c.span*size - c.span*size/5
		want := hexColor(c.color)
		for dy := range sq {
			for dx := range sq {
				x, y := ox+c.x*size+dx, oy+c.y*size+dy
				onHero[y*w+x] = true
				corner := (dx == 0 || dx == sq-1) && (dy == 0 || dy == sq-1)
				if got := img.RGBAAt(x, y); !corner && got != want {
					t.Fatalf("hero pixel %d,%d is %v, want %v", x, y, got, want)
				}
			}
		}
	}
	bg := hexColor(socialBg)
	near := dilate(onHero, w, h, ledgerHaloHard)
	for y := range h {
		for x := range w {
			if c := img.RGBAAt(x, y); near[y*w+x] && !onHero[y*w+x] && c != bg {
				t.Fatalf("pixel %d,%d (%v) is inked within %d px of the hero", x, y, c, ledgerHaloHard)
			}
		}
	}
	// Tiles span 4 columns and 2 rows of the ledger, wider than any gap between
	// entries, so every tile clear of the halo holds ink.
	const tw, th = 4 * ledgerCellW, 2 * ledgerCellH
	far := dilate(onHero, w, h, ledgerHaloSoft)
	tiles, bare := 0, 0
	for ty := 0; ty+th <= h; ty += th {
		for tx := 0; tx+tw <= w; tx += tw {
			beyond, inked := true, false
			for y := ty; y < ty+th; y++ {
				for x := tx; x < tx+tw; x++ {
					beyond = beyond && !far[y*w+x]
					inked = inked || img.RGBAAt(x, y) != bg
				}
			}
			if beyond {
				tiles++
				if !inked {
					bare++
					t.Errorf("tile at %d,%d is bare though clear of the halo", tx, ty)
				}
			}
		}
	}
	if tiles < 100 {
		t.Errorf("only %d tiles clear of the halo: the ledger hardly shows", tiles)
	}
}

// dilate marks every pixel within r px (Chebyshev distance) of a marked one.
func dilate(on []bool, w, h, r int) []bool {
	row := make([]bool, w*h)
	for y := range h {
		for x := range w {
			for d := max(0, x-r); d <= min(w-1, x+r) && !row[y*w+x]; d++ {
				row[y*w+x] = on[y*w+d]
			}
		}
	}
	out := make([]bool, w*h)
	for y := range h {
		for x := range w {
			for d := max(0, y-r); d <= min(h-1, y+r) && !out[y*w+x]; d++ {
				out[y*w+x] = row[d*w+x]
			}
		}
	}
	return out
}

// Every value in the ledger is synthetic: drawn from the seed alone, never
// read from a repository. Same seed, same entries; another seed, others.
func TestLedgerEntriesAreSynthetic(t *testing.T) {
	a, b := ledgerEntries(artSeed, 200), ledgerEntries(artSeed, 200)
	other := ledgerEntries(artSeed+1, 200)
	ids := map[string]bool{}
	id := regexp.MustCompile(`^[0-9a-f]{12}$`)
	for i, e := range a {
		if e.String() != b[i].String() || e.k != b[i].k {
			t.Fatalf("entry %d differs between runs with one seed: %v, %v", i, e, b[i])
		}
		if !id.MatchString(e.id) || ids[e.id] {
			t.Errorf("entry %d id %q is not a fresh 12-digit hex id", i, e.id)
		}
		ids[e.id] = true
		if i > 0 && !e.at.After(a[i-1].at) {
			t.Errorf("entry %d at %v does not follow %v", i, e.at, a[i-1].at)
		}
		if e.id == other[i].id {
			t.Errorf("entry %d id %q ignores the seed", i, e.id)
		}
	}
}

// The ledger lays its entries out in order as gb/1 lines, so the background
// reads as the payload of a mirror commit and as nothing else.
func TestLedgerShowsEntriesInOrder(t *testing.T) {
	const cols, rows = 106, 35
	es := ledgerEntries(artSeed, ledgerCap(cols, rows))
	text := ""
	for _, row := range ledger(cols, rows, es) {
		for _, c := range row {
			text += string(max(c.ch, ' '))
		}
		text += "\n"
	}
	line := regexp.MustCompile(`gb/1 [0-9a-f]{12}  \d{4}-\d{2}-\d{2} \d{2}:\d{2}`)
	full := line.FindAllString(text, -1)
	if len(full) < rows*2 {
		t.Fatalf("%d whole entries on %d rows, want at least 2 a row", len(full), rows)
	}
	next := 0
	for _, f := range full {
		for next < len(es) && es[next].String() != f {
			next++
		}
		if next == len(es) {
			t.Fatalf("%q is not a ledger entry in order", f)
		}
	}
}

// At the small unfurl the ledger's g must not pass for a 9: g sits on the
// x-height like the other lowercase letters and drops below the baseline,
// where no digit reaches.
func TestLedgerGlyphGIsNotNine(t *testing.T) {
	g, nine := ledgerFont['g'], ledgerFont['9']
	if g[0] != "....." || g[1] != "....." {
		t.Errorf("g rises above the x-height: %q", g[:2])
	}
	if g[ledgerBaseline+1] == "....." {
		t.Error("g has no descender")
	}
	for r, glyph := range ledgerFont {
		if r >= '0' && r <= '9' && glyph[ledgerBaseline+1] != "....." {
			t.Errorf("digit %q reaches below the baseline", r)
		}
	}
	if g == nine {
		t.Error("g and 9 share a glyph")
	}
}

// GitHub wants a 1280x640 social preview under 1 MB.
func TestSocialPNGFitsGitHub(t *testing.T) {
	b := socialPNG()
	if len(b) >= 1<<20 {
		t.Errorf("social preview is %d bytes, GitHub's limit is 1 MB", len(b))
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 1280 || cfg.Height != 640 {
		t.Errorf("social preview is %dx%d, want 1280x640", cfg.Width, cfg.Height)
	}
}
