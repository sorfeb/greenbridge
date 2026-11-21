package brand

import (
	"bytes"
	"image"
	"image/color"
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

// The social preview's haze whispers: the bridge and wordmark keep their exact
// colours, no background pixel outshines the dimmest green square, and the
// haze covers the canvas rather than leaving it flat.
func TestSocialHazeStaysBehindHero(t *testing.T) {
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
	dimmest := luma(hexColor(p.green[1]))
	bg, textured := hexColor(socialBg), 0
	for y := range h {
		for x := range w {
			if onHero[y*w+x] {
				continue
			}
			c := img.RGBAAt(x, y)
			if luma(c) >= dimmest {
				t.Fatalf("background pixel %d,%d (%v) outshines the dimmest green %s", x, y, c, p.green[1])
			}
			if c != bg {
				textured++
			}
		}
	}
	if textured < w*h/2 {
		t.Errorf("haze covers %d of %d background pixels, want at least half", textured, w*h)
	}
}

func luma(c color.RGBA) int { return 2126*int(c.R) + 7152*int(c.G) + 722*int(c.B) }

// GitHub rejects a social preview of 1 MB or more.
func TestSocialPNGUnderGitHubLimit(t *testing.T) {
	if n := len(socialPNG()); n >= 1<<20 {
		t.Errorf("social preview is %d bytes, GitHub's limit is 1 MB", n)
	}
}
