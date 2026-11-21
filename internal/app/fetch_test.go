package app

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/gittest"
)

// Commits pushed from another machine stay invisible until the source repo
// fetches. --fetch fetches every source repo before reading it; a repo whose
// fetch fails is still read as it is, and reported, never fatal.
func TestFetchBeforeReading(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)

	upstream := filepath.Join(tmp, "upstream.git")
	gittest.InitBare(t, upstream)
	laptop, desk := filepath.Join(tmp, "laptop"), filepath.Join(tmp, "desk")
	gittest.Git(t, "", nil, "clone", "-q", upstream, laptop)
	gittest.Git(t, "", nil, "clone", "-q", upstream, desk)
	gittest.Commit(t, laptop, "me@work.com", "2025-01-02T10:00:00Z")
	gittest.Git(t, laptop, nil, "push", "-q", "origin", "HEAD:main")

	// broken has a commit of its own and a remote that no longer exists.
	broken := cfg.Repos[0]
	gittest.Commit(t, broken, "me@work.com", "2025-01-03T10:00:00Z")
	gittest.Git(t, broken, nil, "remote", "add", "origin", filepath.Join(tmp, "gone.git"))
	cfg.Repos = append(cfg.Repos, desk)

	p, err := ComputePlan(cfg, testSecret, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.New) != 1 || len(p.FetchFailed()) != 0 {
		t.Fatalf("without --fetch: %d new, fetch failures %v; want only broken's commit and no fetch", len(p.New), p.FetchFailed())
	}

	res, err := Sync(cfg, testSecret, SyncOptions{Fetch: true}, io.Discard)
	if err != nil {
		t.Fatalf("sync --fetch = %v, want a failed fetch to be reported, not fatal", err)
	}
	if res.Added != 2 {
		t.Errorf("sync --fetch added %d, want 2: the pushed commit and broken's own", res.Added)
	}
	if len(res.FetchFailed) != 1 || res.FetchFailed[0].Path != broken {
		t.Fatalf("fetch failures = %+v, want only %s", res.FetchFailed, broken)
	}

	var b strings.Builder
	WriteFetchFailed(&b, res.FetchFailed)
	if got := b.String(); !strings.HasPrefix(got, "  fetch failed "+broken+": ") || strings.Count(got, "\n") != 1 {
		t.Errorf("fetch failure line = %q, want one line naming %s", got, broken)
	}
}
