package app

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/brand"
	"github.com/sorfeb/greenbridge/internal/gittest"
)

// Mirror IDs no source repo on this machine produces (another machine's
// work, a repo dropped from config, an email removed) make "already
// mirrored" exceed "desired". Plan and sync
// say how many, and sync never deletes them.
func TestOrphansReportedNeverDeleted(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	src := cfg.Repos[0]
	gittest.Commit(t, src, "me@work.com", "2025-01-02T10:00:00Z")
	gittest.Commit(t, src, "alt@work.local", "2025-01-03T10:00:00Z")
	gittest.Commit(t, src, "alt@work.local", "2025-01-04T10:00:00Z")
	if res, err := Sync(cfg, testSecret, SyncOptions{}, io.Discard); err != nil || res.Added != 3 || res.Orphans != 0 {
		t.Fatalf("first sync: %+v %v, want 3 added, no orphans", res, err)
	}

	cfg.Emails = []string{"me@work.com"}
	res, err := Sync(cfg, testSecret, SyncOptions{}, io.Discard)
	if err != nil || res.Desired != 1 || res.Ledger != 3 || res.Orphans != 2 || res.Added != 0 {
		t.Fatalf("after removing an email: %+v %v, want 1 desired, 3 mirrored, 2 orphans", res, err)
	}
	if n := gittest.Git(t, cfg.Mirror.Remote, nil, "rev-list", "--count", "main"); n != "3" {
		t.Errorf("remote has %s commits, want all 3 kept", n)
	}
	var b strings.Builder
	WriteOrphans(&b, res.Orphans)
	if want := "2 in mirror not found in source repos on this machine\n"; b.String() != want {
		t.Errorf("orphan line = %q, want %q", b.String(), want)
	}

	p, err := ComputePlan(cfg, testSecret, PlanOptions{})
	if err != nil || p.Orphans != 2 {
		t.Fatalf("plan: %+v %v, want 2 orphans", p, err)
	}
	b.Reset()
	p.Report(&b, time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), 2, brand.Plain)
	if !strings.Contains(b.String(), "already in mirror: 3, would add: 0\n2 in mirror not found in source repos on this machine\n") {
		t.Errorf("report lacks the orphan line:\n%s", b.String())
	}

	// --since leaves older commits out of desired on purpose: they are not
	// orphans, so nothing is reported.
	since := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	if res, err := Sync(cfg, testSecret, SyncOptions{Since: since}, io.Discard); err != nil || res.Orphans != 0 {
		t.Errorf("sync --since: %+v %v, want no orphans reported", res, err)
	}
	if p, err := ComputePlan(cfg, testSecret, PlanOptions{Since: since}); err != nil || p.Orphans != 0 {
		t.Errorf("plan --since: orphans %d %v, want 0", p.Orphans, err)
	}

	var none strings.Builder
	WriteOrphans(&none, 0)
	if none.Len() != 0 {
		t.Errorf("no orphans printed %q, want nothing", none.String())
	}
}
