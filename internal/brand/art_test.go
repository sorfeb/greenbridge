package brand

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
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
