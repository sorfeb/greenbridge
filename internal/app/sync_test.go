package app

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/ident"
)

func TestSyncEndToEnd(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	src, remote := cfg.Repos[0], cfg.Mirror.Remote
	secret := testSecret

	gittest.Commit(t, src, "me@work.com", "2025-01-02T23:30:00+07:00")
	gittest.Commit(t, src, "Me@Work.com", "2025-01-03T10:00:00+07:00")
	gittest.Commit(t, src, "someone@else.com", "2025-01-03T11:00:00+07:00")
	gittest.Commit(t, src, "alt@work.local", "2025-01-04T09:00:00-05:00")

	res, err := Sync(cfg, secret, SyncOptions{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || !res.Pushed {
		t.Fatalf("first sync: %+v, want 3 added and pushed", res)
	}

	log := gittest.Git(t, remote, nil, "log", "--format=%s|%ae|%aI|%cI", "main")
	lines := strings.Split(log, "\n")
	if len(lines) != 3 {
		t.Fatalf("remote has %d commits, want 3:\n%s", len(lines), log)
	}
	for _, l := range lines {
		f := strings.Split(l, "|")
		if !strings.HasPrefix(f[0], "gb/1 ") || f[1] != cfg.MirrorAuthor.Email || f[2] != f[3] {
			t.Errorf("bad mirror commit %q", l)
		}
		if strings.Contains(l, gittest.Message) || strings.Contains(l, "work.com") {
			t.Errorf("mirror leaked source data: %q", l)
		}
	}
	if !strings.Contains(log, "2025-01-02T23:30:00+07:00") || !strings.Contains(log, "2025-01-04T09:00:00-05:00") {
		t.Errorf("original offsets not preserved:\n%s", log)
	}
	if tree := gittest.Git(t, remote, nil, "rev-parse", "main^{tree}"); tree != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Errorf("mirror tree not empty: %s", tree)
	}

	// Idempotent: nothing new.
	if res, err = Sync(cfg, secret, SyncOptions{}, io.Discard); err != nil || res.Added != 0 {
		t.Fatalf("second sync: %+v %v, want 0 added", res, err)
	}

	// A rebase rewrites hashes but keeps author dates: still nothing new.
	gittest.Git(t, src, nil, "-c", "commit.gpgsign=false", "-c", "user.name=w", "-c", "user.email=me@work.com",
		"rebase", "-q", "--force-rebase", "--root", "--committer-date-is-author-date")
	if res, err = Sync(cfg, secret, SyncOptions{}, io.Discard); err != nil || res.Added != 0 {
		t.Fatalf("after rebase: %+v %v, want 0 added", res, err)
	}

	// Another machine pushed meanwhile: the local clone is stale, sync
	// must build on the remote tip rather than fail or duplicate.
	other := *cfg
	other.Mirror.Path = filepath.Join(tmp, "mirror2.git")
	gittest.Commit(t, src, "me@work.com", "2025-02-01T12:00:00+07:00")
	if res, err = Sync(&other, secret, SyncOptions{}, io.Discard); err != nil || res.Added != 1 {
		t.Fatalf("second machine: %+v %v, want 1 added", res, err)
	}
	gittest.Commit(t, src, "me@work.com", "2025-02-02T12:00:00+07:00")
	if res, err = Sync(cfg, secret, SyncOptions{}, io.Discard); err != nil || res.Added != 1 || !res.Pushed {
		t.Fatalf("first machine after race: %+v %v, want 1 added", res, err)
	}
	if n := gittest.Git(t, remote, nil, "rev-list", "--count", "main"); n != "5" {
		t.Fatalf("remote has %s commits, want 5", n)
	}

	// Dry run writes nothing.
	gittest.Commit(t, src, "me@work.com", "2025-03-01T12:00:00+07:00")
	if res, err = Sync(cfg, secret, SyncOptions{DryRun: true}, io.Discard); err != nil || res.Added != 1 || res.Pushed {
		t.Fatalf("dry run: %+v %v", res, err)
	}
	if n := gittest.Git(t, remote, nil, "rev-list", "--count", "main"); n != "5" {
		t.Fatalf("dry run pushed: %s commits", n)
	}
}

func TestDateOnlyPolicy(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	gittest.Init(t, src)
	gittest.Commit(t, src, "me@work.com", "2025-01-02T23:30:00+07:00")
	cfg := &config.Config{
		Emails: []string{"me@work.com"}, Repos: []string{src},
		TimePolicy: ident.PolicyDateOnly, Identity: ident.ModeAuthorDate,
	}
	d, err := Collect(cfg, testSecret, PlanOptions{})
	if err != nil || len(d.Entries) != 1 {
		t.Fatalf("collect: %v %v", d, err)
	}
	if e := d.Entries[0].Time; DayKey(e) != "2025-01-02" || e.Location() != time.UTC {
		t.Fatalf("date-only stamp %v", e)
	}
}
