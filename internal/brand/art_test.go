package brand

import (
	"bytes"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
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
