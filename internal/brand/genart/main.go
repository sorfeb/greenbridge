// Command genart writes the brand assets (README art, social preview) to
// .github/assets. Run it through go generate, which starts it in
// internal/brand:
//
//	go generate ./internal/brand
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sorfeb/greenbridge/internal/brand"
)

const dir = "../../.github/assets"

func main() {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, data := range brand.Assets() {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
