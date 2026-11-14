package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// seenFile is the record of repos found under scan folders. It holds local
// paths only and never leaves the machine.
type seenFile struct {
	Repos []string `json:"repos"`
}

// SeenPath is where sync records the repos it found under scan folders.
func SeenPath() (string, error) {
	d, err := Dir()
	return filepath.Join(d, "seen.json"), err
}

// LoadSeen returns the recorded repos, keyed by cleaned path. A missing
// record is empty.
func LoadSeen() (map[string]bool, error) {
	p, err := SeenPath()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return seen, nil
	}
	if err != nil {
		return nil, err
	}
	var f seenFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	for _, r := range f.Repos {
		seen[filepath.Clean(r)] = true
	}
	return seen, nil
}

// AddSeen adds repos to the record.
func AddSeen(repos []string) error {
	if len(repos) == 0 {
		return nil
	}
	seen, err := LoadSeen()
	if err != nil {
		return err
	}
	for _, r := range repos {
		seen[filepath.Clean(r)] = true
	}
	f := seenFile{Repos: make([]string, 0, len(seen))}
	for r := range seen {
		f.Repos = append(f.Repos, r)
	}
	slices.Sort(f.Repos)
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	p, err := SeenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}
