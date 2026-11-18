package app

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/brand"
	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/ident"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden from the code")

var testSecret = bytes.Repeat([]byte{7}, ident.SecretSize)

// mirrored returns a config with one source repo at tmp/src and a private
// mirror pushing to the bare repo tmp/remote.git. The greenbridge home is
// tmp/home.
func mirrored(t *testing.T, tmp string) *config.Config {
	t.Helper()
	t.Setenv("GBRIDGE_HOME", filepath.Join(tmp, "home"))
	gittest.Init(t, filepath.Join(tmp, "src"))
	gittest.InitBare(t, filepath.Join(tmp, "remote.git"))
	cfg := &config.Config{
		Version: 1,
		Emails:  []string{"me@work.com", "alt@work.local"},
		Repos:   []string{filepath.Join(tmp, "src")},
		Mirror: config.MirrorSettings{
			Remote: filepath.Join(tmp, "remote.git"), Path: filepath.Join(tmp, "mirror.git"),
			Branch: "main", Visibility: "private",
		},
		MirrorAuthor: config.MirrorAuthor{Name: "me", Email: "1+me@users.noreply.github.com"},
		TimePolicy:   ident.PolicyExact,
		Identity:     ident.ModeAuthorDate,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/app -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs (run go test ./internal/app -update and review the diff)\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

// The plan report is what a user reads before anything is published: repo
// lines, totals, mirror state, year table, and graph.
func TestPlanReport(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	src := cfg.Repos[0]

	gittest.Commit(t, src, "me@work.com", "2024-12-30T18:00:00+07:00")
	gittest.Commit(t, src, "me@work.com", "2025-01-02T23:30:00+07:00")
	gittest.Commit(t, src, "Me@Work.com", "2025-01-03T10:00:00+07:00")
	gittest.Commit(t, src, "someone@else.com", "2025-01-03T11:00:00+07:00")
	if _, err := Sync(cfg, testSecret, SyncOptions{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"09", "11", "14"} {
		gittest.Commit(t, src, "me@work.com", "2025-02-10T"+h+":00:00+07:00")
	}
	gittest.Commit(t, src, "alt@work.local", "2025-03-11T09:00:00-05:00")

	// A clone holds the same commits: matched twice, mirrored once.
	clone := filepath.Join(tmp, "clone")
	gittest.Git(t, "", nil, "clone", "-q", src, clone)
	hosted := filepath.Join(tmp, "hosted")
	gittest.Init(t, hosted)
	gittest.Git(t, hosted, nil, "remote", "add", "origin", "https://github.com/acme/site.git")
	cfg.Repos = append(cfg.Repos, clone, hosted, filepath.Join(tmp, "missing"))

	p, err := ComputePlan(cfg, testSecret, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	p.Report(&b, time.Date(2025, 3, 12, 15, 0, 0, 0, time.UTC), 53, brand.Plain)
	golden(t, "plan.golden", filepath.ToSlash(strings.ReplaceAll(b.String(), tmp, "$TMP")))
}

// When the mirror can't be fetched, plan and sync see the same ledger: the
// one from the last successful fetch. Plan reports it with a warning; sync
// writes nothing, because publishing needs the current remote tip.
func TestPlanAndSyncAgreeWhenFetchFails(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	src := cfg.Repos[0]

	gittest.Commit(t, src, "me@work.com", "2025-01-02T12:00:00+07:00")
	gittest.Commit(t, src, "me@work.com", "2025-01-03T12:00:00+07:00")
	if res, err := Sync(cfg, testSecret, SyncOptions{}, io.Discard); err != nil || res.Added != 2 || !res.Pushed {
		t.Fatalf("first sync: %+v %v, want 2 added and pushed", res, err)
	}
	gittest.Commit(t, src, "me@work.com", "2025-01-04T12:00:00+07:00")

	remote := cfg.Mirror.Remote
	if err := os.Rename(remote, remote+".gone"); err != nil {
		t.Fatal(err)
	}
	local := gittest.Git(t, cfg.Mirror.Path, nil, "rev-parse", "refs/heads/main")

	p, err := ComputePlan(cfg, testSecret, PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.FetchErr == nil || p.Ledger != 2 || len(p.New) != 1 {
		t.Fatalf("plan: fetch error %v, ledger %d, new %d; want an error, 2, 1", p.FetchErr, p.Ledger, len(p.New))
	}
	var b strings.Builder
	p.Report(&b, time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), 4, brand.Plain)
	for _, want := range []string{"mirror fetch failed", "counts are from the last fetch", "already in mirror: 2, would add: 1"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, b.String())
		}
	}

	for _, opt := range []SyncOptions{{}, {DryRun: true}, {NoPush: true}} {
		if res, err := Sync(cfg, testSecret, opt, io.Discard); err == nil || !strings.Contains(err.Error(), "fetch") {
			t.Errorf("sync %+v: %+v %v, want a fetch error", opt, res, err)
		}
	}
	if got := gittest.Git(t, cfg.Mirror.Path, nil, "rev-parse", "refs/heads/main"); got != local {
		t.Errorf("sync wrote to the mirror after a failed fetch: %s, was %s", got, local)
	}

	// --offline never contacts the remote, so there is nothing to warn about.
	off, err := ComputePlan(cfg, testSecret, PlanOptions{Offline: true})
	if err != nil || off.FetchErr != nil || off.Ledger != 2 || len(off.New) != 1 {
		t.Fatalf("offline plan: %+v %v, want ledger 2, new 1, no fetch error", off, err)
	}
}
