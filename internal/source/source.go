// Package source reads commit metadata from local clones. It never reads
// diffs, file contents, commit messages, or branch names.
package source

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// Read returns the activity in repo: commits authored by any of emails, on any branch,
// remote-tracking branch, or tag. Stash and notes refs are excluded.
// A zero since means all history.
func Read(repo string, emails []string, since time.Time) ([]Activity, error) {
	want := map[string]bool{}
	for _, e := range emails {
		want[normEmail(e)] = true
	}
	// Author filtering happens here, not via --author, which takes a regex
	// and misreads emails containing "+" or ".". --since is also avoided:
	// it filters by committer date and stops early on out-of-order history.
	lines, err := logFields(repo, "%H", "%ae", "%aI")
	if err != nil {
		return nil, err
	}
	var acts []Activity
	for _, f := range lines {
		if !want[normEmail(f[1])] {
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

// Author is one author identity in a repo and the commits it made.
type Author struct {
	Name    string
	Email   string // lowercased
	Commits int
}

// Authors counts commits per author name and email on the refs Read reads,
// authored on or after since. A zero since means all history. Sorted by
// email, then name.
func Authors(repo string, since time.Time) ([]Author, error) {
	lines, err := logFields(repo, "%an", "%ae", "%aI")
	if err != nil {
		return nil, err
	}
	type key struct{ name, email string }
	counts := map[key]int{}
	for _, f := range lines {
		t, err := time.Parse(time.RFC3339, f[2])
		if err != nil {
			return nil, fmt.Errorf("%s: bad author date %q", repo, f[2])
		}
		if !since.IsZero() && t.Before(since) {
			continue
		}
		counts[key{strings.TrimSpace(f[0]), normEmail(f[1])}]++
	}
	authors := make([]Author, 0, len(counts))
	for k, n := range counts {
		authors = append(authors, Author{Name: k.name, Email: k.email, Commits: n})
	}
	slices.SortFunc(authors, func(a, b Author) int {
		return cmp.Or(strings.Compare(a.Email, b.Email), strings.Compare(a.Name, b.Name))
	})
	return authors, nil
}

// logFields runs git log over branches, remote-tracking branches and tags,
// one commit per line split into the given format fields. Every other ref is
// left out: stash, notes, and tool refs such as editor checkpoints under
// refs/t3/, which hold commits the user never made.
func logFields(repo string, fields ...string) ([][]string, error) {
	out, err := gitx.Git(repo, "log", "--branches", "--remotes", "--tags", "--no-color",
		"--format="+strings.Join(fields, "%x1f"))
	if err != nil {
		return nil, err
	}
	var lines [][]string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if f := strings.Split(strings.TrimRight(line, "\r"), "\x1f"); len(f) == len(fields) {
			lines = append(lines, f)
		}
	}
	return lines, nil
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
