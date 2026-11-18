// Package source reads commit metadata from local clones. It never reads
// diffs, file contents, commit messages, or branch names.
package source

import (
	"cmp"
	"errors"
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

// ErrUnreadable matches the error Inspect returns when repo is not a
// readable git repo. The error's text stays git's own.
var ErrUnreadable = errors.New("not a readable git repo")

type unreadableError struct{ err error }

func (e unreadableError) Error() string        { return e.err.Error() }
func (e unreadableError) Unwrap() error        { return e.err }
func (e unreadableError) Is(target error) bool { return target == ErrUnreadable }

// InspectOptions controls Inspect.
type InspectOptions struct {
	// SkipGitHub leaves a repo with a github.com remote unread, so it costs
	// one git run: GitHub already counts it.
	SkipGitHub bool
	// Fetch runs git fetch on the repo's remotes before reading it.
	Fetch bool
}

// Inspection is what one source repo holds.
type Inspection struct {
	GitHubRemote bool     // a remote points at github.com
	FetchErr     error    // asked to fetch and it failed; History is what the repo already had
	History      *History // nil when the repo has a github.com remote and SkipGitHub is set
}

// Inspect checks repo's remotes, fetches when asked, then reads its history
// in one git log. A failed fetch is recorded in FetchErr, not returned.
func Inspect(repo string, opt InspectOptions) (Inspection, error) {
	gh, err := hasGitHubRemote(repo)
	if err != nil {
		return Inspection{}, unreadableError{err}
	}
	in := Inspection{GitHubRemote: gh}
	if gh && opt.SkipGitHub {
		return in, nil
	}
	if opt.Fetch {
		in.FetchErr = fetch(repo)
	}
	in.History, err = readHistory(repo)
	return in, err
}

// fetch runs git fetch on every remote of repo. It never prunes, so commits
// on branches deleted upstream stay readable, and git never prompts for
// credentials: a remote that needs them fails instead of stalling a run that
// fetches many repos at once. The error is git's own reason, on one line.
func fetch(repo string) error {
	env := []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}
	if _, err := gitx.Run(repo, env, nil, "fetch", "--all", "--no-prune", "--quiet"); err != nil {
		return errors.New(gitReason(err))
	}
	return nil
}

// gitReason returns the first "fatal:" or "error:" line of a git error, else
// its first line.
func gitReason(err error) string {
	msg := err.Error()
	for line := range strings.SplitSeq(msg, "\n") {
		for _, p := range []string{"fatal: ", "error: "} {
			if _, reason, ok := strings.Cut(line, p); ok {
				return strings.TrimSpace(reason)
			}
		}
	}
	first, _, _ := strings.Cut(msg, "\n")
	return first
}

// History is every commit on a repo's branches, remote-tracking branches and
// tags. Every other ref is left out: stash, notes, and tool refs such as
// editor checkpoints under refs/t3/, which hold commits the user never made.
// It holds no messages, paths, or diffs, and never leaves the machine.
type History struct {
	repo    string
	commits []commit
}

type commit struct {
	sha, name, email string // email lowercased, name trimmed
	time             time.Time
}

// readHistory runs one git log over repo.
func readHistory(repo string) (*History, error) {
	// --author is avoided: it takes a regex and misreads emails containing
	// "+" or ".". --since is avoided too: it filters by committer date and
	// stops early on out-of-order history. Both filters run on the result.
	out, err := gitx.Git(repo, "log", "--branches", "--remotes", "--tags", "--no-color",
		"--format=%H%x1f%an%x1f%ae%x1f%aI")
	if err != nil {
		return nil, err
	}
	h := &History{repo: repo}
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\x1f")
		if len(f) != 4 {
			continue
		}
		t, err := time.Parse(time.RFC3339, f[3])
		if err != nil {
			return nil, fmt.Errorf("%s: commit %s: bad author date %q", repo, f[0], f[3])
		}
		h.commits = append(h.commits, commit{f[0], strings.TrimSpace(f[1]), normEmail(f[2]), t})
	}
	return h, nil
}

// Activity returns the commits authored by any of emails on or after since.
// A zero since means all history.
func (h *History) Activity(emails []string, since time.Time) []Activity {
	want := map[string]bool{}
	for _, e := range emails {
		want[normEmail(e)] = true
	}
	var acts []Activity
	for _, c := range h.commits {
		if want[c.email] && !before(c.time, since) {
			acts = append(acts, Activity{SHA: c.sha, Email: c.email, Repo: h.repo, Time: c.time})
		}
	}
	return acts
}

// Author is one author identity in a source repo and the commits it made.
type Author struct {
	Name    string
	Email   string // lowercased
	Commits int
}

// Authors counts commits per author name and email on or after since. A zero
// since means all history. Sorted by email, then name.
func (h *History) Authors(since time.Time) []Author {
	type key struct{ name, email string }
	counts := map[key]int{}
	for _, c := range h.commits {
		if !before(c.time, since) {
			counts[key{c.name, c.email}]++
		}
	}
	authors := make([]Author, 0, len(counts))
	for k, n := range counts {
		authors = append(authors, Author{Name: k.name, Email: k.email, Commits: n})
	}
	slices.SortFunc(authors, func(a, b Author) int {
		return cmp.Or(strings.Compare(a.Email, b.Email), strings.Compare(a.Name, b.Name))
	})
	return authors
}

func before(t, since time.Time) bool { return !since.IsZero() && t.Before(since) }

// hasGitHubRemote reports whether any remote points at github.com. GitHub
// already counts pushes to such repos, so mirroring them double counts.
func hasGitHubRemote(repo string) (bool, error) {
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
