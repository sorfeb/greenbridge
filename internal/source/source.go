// Package source reads commit metadata from local clones. It never reads
// diffs, file contents, commit messages, or branch names.
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/gitx"
)

// Activity is one authored commit. SHA and Email feed the identity hash and
// never leave the machine.
type Activity struct {
	SHA   string
	Email string
	Repo  string
	Time  time.Time // author date with its original UTC offset
}

// Scan returns commits in repo authored by any of emails, on any branch,
// remote-tracking branch, or tag. Stash and notes refs are excluded.
// A zero since means all history.
func Scan(repo string, emails []string, since time.Time) ([]Activity, error) {
	want := map[string]bool{}
	for _, e := range emails {
		want[normEmail(e)] = true
	}
	// Author filtering happens here, not via --author, which takes a regex
	// and misreads emails containing "+" or ".". --since is also avoided:
	// it filters by committer date and stops early on out-of-order history.
	out, err := gitx.Git(repo, "log", "--branches", "--remotes", "--tags", "--no-color", "--format=%H%x1f%ae%x1f%aI")
	if err != nil {
		return nil, err
	}
	var acts []Activity
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x1f")
		if len(f) != 3 || !want[normEmail(f[1])] {
			continue
		}
		t, err := time.Parse(time.RFC3339, f[2])
		if err != nil {
			return nil, fmt.Errorf("%s: commit %s: bad author date %q", repo, f[0], f[2])
		}
		if !since.IsZero() && t.Before(since) {
			continue
		}
		acts = append(acts, Activity{SHA: f[0], Email: normEmail(f[1]), Repo: repo, Time: t})
	}
	return acts, nil
}

// HasGitHubRemote reports whether any remote points at github.com. GitHub
// already counts pushes to such repos, so mirroring them double counts.
func HasGitHubRemote(repo string) (bool, error) {
	out, err := gitx.Git(repo, "remote", "-v")
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(out), "github.com"), nil
}

// FindRepos walks root up to maxDepth levels and returns directories that
// contain a .git entry. It does not descend into repos it finds.
func FindRepos(root string, maxDepth int) []string {
	var found []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			found = append(found, dir)
			return
		}
		if depth >= maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				continue
			}
			walk(filepath.Join(dir, name), depth+1)
		}
	}
	walk(root, 0)
	return found
}

func normEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
}
