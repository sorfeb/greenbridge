// Package site holds the project homepage. The Pages workflow publishes
// index.html with .github/assets copied to assets/.
package site

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func page(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// An art rename in internal/brand must not leave the homepage with broken images.
func TestAssetsExist(t *testing.T) {
	refs := regexp.MustCompile(`assets/([\w.-]+)`).FindAllStringSubmatch(page(t), -1)
	if len(refs) == 0 {
		t.Fatal("no assets/ references; the page lost its art")
	}
	for _, m := range refs {
		if _, err := os.Stat(filepath.Join("..", ".github", "assets", m[1])); err != nil {
			t.Errorf("index.html references assets/%s: %v", m[1], err)
		}
	}
}

// The page says it has no trackers: it loads nothing from another origin.
func TestNoThirdPartyLoads(t *testing.T) {
	p := page(t)
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?:src|srcset)="(?:https?:)?//[^"]*"`),
		regexp.MustCompile(`<link\b[^>]*\brel="(?:stylesheet|preload|modulepreload|icon|preconnect)"[^>]*href="(?:https?:)?//[^"]*"`),
		regexp.MustCompile(`@import|url\((?:["']?)(?:https?:)?//`),
	} {
		for _, m := range re.FindAllString(p, -1) {
			t.Errorf("third-party load: %s", m)
		}
	}
}
