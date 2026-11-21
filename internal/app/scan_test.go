package app

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/brand"
	"github.com/sorfeb/greenbridge/internal/gittest"
)

func newPaths(rs []RepoStat) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Path)
	}
	return out
}

// A repo cloned under a scan folder after setup is mirrored on the next plan
// and sync, and both say it is new until a sync records it. Found repos
// qualify only with commits by a configured email on branches, remotes, or
// tags.
func TestScanFolderFindsLaterClones(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	work := filepath.Join(tmp, "work")
	api, web := filepath.Join(work, "api"), filepath.Join(work, "team", "web")
	theirs, t3only, hosted := filepath.Join(work, "theirs"), filepath.Join(work, "t3only"), filepath.Join(work, "hosted")
	for _, r := range []string{api, web, theirs, t3only, hosted} {
		gittest.Init(t, r)
	}
	gittest.Commit(t, api, "me@work.com", "2025-01-02T10:00:00Z")
	gittest.Commit(t, web, "me@work.com", "2025-01-03T10:00:00Z")
	gittest.Commit(t, theirs, "someone@else.com", "2025-01-03T10:00:00Z")
	gittest.Commit(t, hosted, "me@work.com", "2025-01-03T11:00:00Z")
	gittest.Git(t, hosted, nil, "remote", "add", "origin", "https://github.com/acme/site.git")
	// T3 Code keeps checkpoint commits under refs/t3/*; they are not work.
	gittest.Commit(t, t3only, "me@work.com", "2025-01-03T12:00:00Z")
	gittest.Git(t, t3only, nil, "update-ref", "refs/t3/checkpoint", "HEAD")
	gittest.Git(t, t3only, nil, "update-ref", "-d", "refs/heads/main")
	if err := os.MkdirAll(filepath.Join(work, "notes", "drafts"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Listed explicitly and found by the scan: counted once.
	cfg.Repos = append(cfg.Repos, api+string(filepath.Separator))
	cfg.ScanFolders = []string{work, filepath.Join(tmp, "gone")}

	res, err := Sync(cfg, testSecret, SyncOptions{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := newPaths(res.NewRepos); res.Added != 2 || !slices.Equal(got, []string{web}) {
		t.Fatalf("first sync added %d, new repos %q; want 2 and [%s]", res.Added, got, web)
	}

	later := filepath.Join(work, "later")
	gittest.Init(t, later)
	gittest.Commit(t, later, "Me@Work.com", "2025-01-04T10:00:00Z")

	p, err := ComputePlan(cfg, testSecret, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := newPaths(p.NewRepos()); len(p.New) != 1 || !slices.Equal(got, []string{later}) {
		t.Fatalf("plan would add %d, new repos %q; want 1 and [%s]", len(p.New), got, later)
	}
	var b strings.Builder
	p.Report(&b, time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), 2, brand.Plain)
	golden(t, "scan.golden", filepath.ToSlash(strings.ReplaceAll(b.String(), tmp, "$TMP")))

	// A dry run says the same and records nothing.
	for _, opt := range []SyncOptions{{DryRun: true}, {}} {
		res, err := Sync(cfg, testSecret, opt, io.Discard)
		if got := newPaths(res.NewRepos); err != nil || res.Added != 1 || !slices.Equal(got, []string{later}) {
			t.Fatalf("sync %+v: %+v %v; want 1 added, new repos [%s]", opt, res, err, later)
		}
	}
	res, err = Sync(cfg, testSecret, SyncOptions{}, io.Discard)
	if err != nil || res.Added != 0 || len(res.NewRepos) != 0 {
		t.Fatalf("third sync: %+v %v; want nothing added and no new repos", res, err)
	}
}

// The new-repo line is the same in plan and sync, and plain text.
func TestWriteNewRepos(t *testing.T) {
	var b strings.Builder
	WriteNewRepos(&b, []RepoStat{{Path: "/work/later", Matched: 3, ScanFolder: "/work"}})
	if want := "  new   /work/later (from scan folder /work)\n"; b.String() != want {
		t.Errorf("got %q, want %q", b.String(), want)
	}
}
