package mirror

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/sorfeb/greenbridge/internal/gittest"
)

const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

var author = Author{Name: "me", Email: "1+me@users.noreply.github.com"}

func entry(id, date string) Entry {
	t, err := time.Parse(time.RFC3339, date)
	if err != nil {
		panic(err)
	}
	return Entry{ID: id, Time: t}
}

var (
	e1 = entry("aaaaaaaaaaa1", "2025-01-02T23:30:00+07:00")
	e2 = entry("aaaaaaaaaaa2", "2025-01-03T10:00:00-05:00")
	e3 = entry("aaaaaaaaaaa3", "2025-01-04T09:00:00Z")
)

// remoteClone returns a mirror clone at tmp/<name>.git pushing to the bare
// repo tmp/remote.git, created on first use.
func remoteClone(t *testing.T, tmp, name string) Clone {
	t.Helper()
	remote := filepath.Join(tmp, "remote.git")
	if _, err := os.Stat(remote); os.IsNotExist(err) {
		gittest.InitBare(t, remote)
	}
	return Clone{Path: filepath.Join(tmp, name+".git"), Remote: remote, Branch: "main"}
}

func open(t *testing.T, c Clone) *Ledger {
	t.Helper()
	l, err := c.Open()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// remoteLog returns subject|author email|author date|committer date per
// commit on the remote's main, newest first.
func remoteLog(t *testing.T, c Clone) []string {
	t.Helper()
	out := gittest.Git(t, c.Remote, nil, "log", "--format=%s|%ae|%aI|%cI", "main")
	return strings.Split(out, "\n")
}

func TestPublishWritesOneEmptyCommitPerMissingEntry(t *testing.T) {
	gittest.Need(t)
	c := remoteClone(t, t.TempDir(), "mirror")

	l := open(t, c)
	if l.FetchErr != nil || len(l.IDs) != 0 {
		t.Fatalf("fresh mirror: %+v, want an empty ledger", l)
	}
	res, err := c.Publish(l, []Entry{e1, e2}, author, PublishOptions{}, nil)
	if err != nil || res != (Published{Ledger: 0, Added: 2, Pushed: true}) {
		t.Fatalf("Publish() = %+v, %v; want 2 added and pushed", res, err)
	}
	want := []string{
		"gb/1 aaaaaaaaaaa2|" + author.Email + "|2025-01-03T10:00:00-05:00|2025-01-03T10:00:00-05:00",
		"gb/1 aaaaaaaaaaa1|" + author.Email + "|2025-01-02T23:30:00+07:00|2025-01-02T23:30:00+07:00",
	}
	if got := remoteLog(t, c); !slices.Equal(got, want) {
		t.Errorf("remote log = %q, want %q", got, want)
	}
	if tree := gittest.Git(t, c.Remote, nil, "rev-parse", "main^{tree}"); tree != emptyTree {
		t.Errorf("mirror tree = %s, want the empty tree", tree)
	}

	// Reopened, the ledger holds both IDs and only the new entry is added.
	l = open(t, c)
	if len(l.IDs) != 2 || !l.IDs[e1.ID] || !l.IDs[e2.ID] {
		t.Fatalf("ledger after publish = %v, want %s and %s", l.IDs, e1.ID, e2.ID)
	}
	if m := l.Missing([]Entry{e1, e2, e3}); !slices.Equal(m, []Entry{e3}) {
		t.Errorf("Missing() = %v, want only %s", m, e3.ID)
	}
	res, err = c.Publish(l, []Entry{e1, e2}, author, PublishOptions{}, nil)
	if err != nil || res != (Published{Ledger: 2}) {
		t.Errorf("republish = %+v, %v; want nothing added", res, err)
	}
}

// Another machine sharing the mirror repo pushed after this one opened its
// ledger. The push is rejected, never forced: Publish refetches, recomputes
// what is missing, and builds on the new tip without duplicating.
func TestPublishRebuildsOnARacingPush(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	a, b := remoteClone(t, tmp, "a"), remoteClone(t, tmp, "b")

	stale := open(t, a)
	// e3 alone, so b's root commit differs from the one a would write.
	if _, err := b.Publish(open(t, b), []Entry{e3}, author, PublishOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	res, err := a.Publish(stale, []Entry{e1, e2, e3}, author, PublishOptions{}, &log)
	if err != nil || res != (Published{Ledger: 1, Added: 2, Pushed: true}) {
		t.Fatalf("Publish() after race = %+v, %v; want ledger 1, 2 added, pushed", res, err)
	}
	if !strings.Contains(log.String(), "push rejected, refetching (attempt 1)") {
		t.Errorf("log = %q, want the rejected push reported", log.String())
	}
	if got := remoteLog(t, a); len(got) != 3 || !strings.HasPrefix(got[2], "gb/1 "+e3.ID) {
		t.Errorf("remote log = %q, want e3 from the other machine, then e1 and e2", got)
	}
	if stale.Missing([]Entry{e1, e2, e3}) != nil {
		t.Errorf("ledger after publish = %v, want it to hold every published ID", stale.IDs)
	}
}

// rejectPushes installs a pre-receive hook on the remote that rejects the
// first n pushes.
func rejectPushes(t *testing.T, remote string, n int) {
	t.Helper()
	hook := "#!/bin/sh\n" +
		"count=$(cat rejected 2>/dev/null || echo 0)\n" +
		"if [ \"$count\" -lt " + strconv.Itoa(n) + " ]; then echo $((count+1)) > rejected; echo rejected >&2; exit 1; fi\n"
	if err := os.WriteFile(filepath.Join(remote, "hooks", "pre-receive"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestPublishRetriesARejectedPush(t *testing.T) {
	gittest.Need(t)
	c := remoteClone(t, t.TempDir(), "mirror")
	rejectPushes(t, c.Remote, 1)

	var log strings.Builder
	res, err := c.Publish(open(t, c), []Entry{e1}, author, PublishOptions{}, &log)
	if err != nil || !res.Pushed || res.Added != 1 {
		t.Fatalf("Publish() = %+v, %v; want the second attempt to push", res, err)
	}
	if n := strings.Count(log.String(), "push rejected"); n != 1 {
		t.Errorf("log reports %d rejected pushes, want 1:\n%s", n, log.String())
	}
	if got := remoteLog(t, c); len(got) != 1 {
		t.Errorf("remote log = %q, want one commit", got)
	}
}

func TestPublishGivesUpAfterThreeRejectedPushes(t *testing.T) {
	gittest.Need(t)
	c := remoteClone(t, t.TempDir(), "mirror")
	rejectPushes(t, c.Remote, 9)

	var log strings.Builder
	_, err := c.Publish(open(t, c), []Entry{e1}, author, PublishOptions{}, &log)
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("Publish() error = %v, want it to give up after 3 attempts", err)
	}
	if n := strings.Count(log.String(), "push rejected"); n != 3 {
		t.Errorf("log reports %d rejected pushes, want 3:\n%s", n, log.String())
	}
	if out := gittest.Git(t, c.Remote, nil, "for-each-ref"); out != "" {
		t.Errorf("remote gained refs %q, want none", out)
	}
}

// Entries built on a stale tip either duplicate what another machine pushed
// or get rejected, so a failed fetch writes nothing, not even locally.
func TestPublishRefusesAfterAFailedFetch(t *testing.T) {
	gittest.Need(t)
	c := remoteClone(t, t.TempDir(), "mirror")
	if _, err := c.Publish(open(t, c), []Entry{e1}, author, PublishOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	local := gittest.Git(t, c.Path, nil, "rev-parse", "refs/heads/main")
	if err := os.Rename(c.Remote, c.Remote+".gone"); err != nil {
		t.Fatal(err)
	}

	l := open(t, c)
	if l.FetchErr == nil || len(l.IDs) != 1 || !l.IDs[e1.ID] {
		t.Fatalf("ledger after a failed fetch = %+v, want the last fetched IDs and a fetch error", l)
	}
	for _, opt := range []PublishOptions{{}, {DryRun: true}, {NoPush: true}} {
		if res, err := c.Publish(l, []Entry{e1, e2}, author, opt, nil); err == nil || !strings.Contains(err.Error(), "fetch") {
			t.Errorf("Publish(%+v) = %+v, %v; want a fetch error", opt, res, err)
		}
	}
	if got := gittest.Git(t, c.Path, nil, "rev-parse", "refs/heads/main"); got != local {
		t.Errorf("local branch moved to %s after a failed fetch, was %s", got, local)
	}
}

func TestPublishDryRunAndNoPush(t *testing.T) {
	gittest.Need(t)
	c := remoteClone(t, t.TempDir(), "mirror")

	res, err := c.Publish(open(t, c), []Entry{e1}, author, PublishOptions{DryRun: true}, nil)
	if err != nil || res != (Published{Added: 1}) {
		t.Fatalf("dry run = %+v, %v; want 1 to add, nothing pushed", res, err)
	}
	if out := gittest.Git(t, c.Path, nil, "for-each-ref", "refs/heads"); out != "" {
		t.Errorf("dry run wrote local refs %q", out)
	}

	res, err = c.Publish(open(t, c), []Entry{e1}, author, PublishOptions{NoPush: true}, nil)
	if err != nil || res != (Published{Added: 1}) {
		t.Fatalf("no push = %+v, %v; want 1 added locally, not pushed", res, err)
	}
	if got := gittest.Git(t, c.Path, nil, "log", "--format=%s", "refs/heads/main"); got != "gb/1 "+e1.ID {
		t.Errorf("local branch = %q, want the new mirror commit", got)
	}
	if out := gittest.Git(t, c.Remote, nil, "for-each-ref"); out != "" {
		t.Errorf("no push reached the remote: %q", out)
	}
}

// A local-only mirror clone has no remote: its own branch is the ledger.
func TestPublishLocalOnly(t *testing.T) {
	gittest.Need(t)
	c := Clone{Path: filepath.Join(t.TempDir(), "mirror.git"), Branch: "main"}

	res, err := c.Publish(open(t, c), []Entry{e1, e2}, author, PublishOptions{}, nil)
	if err != nil || res != (Published{Added: 2}) {
		t.Fatalf("Publish() = %+v, %v; want 2 added, nothing pushed", res, err)
	}
	if l := open(t, c); l.FetchErr != nil || len(l.IDs) != 2 {
		t.Errorf("reopened ledger = %+v, want 2 IDs", l)
	}
}

// Author fields cannot break out of the fast-import header.
func TestPublishCleansAuthor(t *testing.T) {
	gittest.Need(t)
	c := Clone{Path: filepath.Join(t.TempDir(), "mirror.git"), Branch: "main"}
	odd := Author{Name: "me <x>\nfoo", Email: "<1+me@users.noreply.github.com>"}
	if _, err := c.Publish(open(t, c), []Entry{e1}, odd, PublishOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, c.Path, nil, "log", "--format=%an|%ae", "refs/heads/main"); got != "me x foo|1+me@users.noreply.github.com" {
		t.Errorf("author = %q", got)
	}
}

// Orphans are mirror IDs no desired entry produces any more.
func TestLedgerOrphans(t *testing.T) {
	l := &Ledger{IDs: map[string]bool{e1.ID: true, e2.ID: true, e3.ID: true}}
	if n := l.Orphans([]Entry{e1, e2, e3}); n != 0 {
		t.Errorf("Orphans() with every ID desired = %d, want 0", n)
	}
	if n := l.Orphans([]Entry{e2, entry("bbbbbbbbbbb1", "2025-02-01T00:00:00Z")}); n != 2 {
		t.Errorf("Orphans() = %d, want 2 (e1 and e3)", n)
	}
}

// traceGit records every git command run from here on and returns a function
// that reads the record and clears it.
func traceGit(t *testing.T) func() string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", path)
	return func() string {
		b, _ := os.ReadFile(path)
		_ = os.Remove(path)
		return string(b)
	}
}

// Open asks the mirror repo for its branch tip first, and fetches only when
// the tip differs from the one the mirror clone already has.
func TestOpenFetchesOnlyWhenTheTipMoved(t *testing.T) {
	gittest.Need(t)
	tmp := t.TempDir()
	a, b := remoteClone(t, tmp, "a"), remoteClone(t, tmp, "b")
	if _, err := a.Publish(open(t, a), []Entry{e1}, author, PublishOptions{}, nil); err != nil {
		t.Fatal(err)
	}

	trace := traceGit(t)
	l := open(t, a)
	if got := trace(); strings.Contains(got, "git fetch") || !strings.Contains(got, "git ls-remote") {
		t.Errorf("reopening after its own push ran:\n%s\nwant ls-remote and no fetch", got)
	}
	if l.FetchErr != nil || len(l.IDs) != 1 || !l.IDs[e1.ID] {
		t.Fatalf("ledger with an unchanged tip = %+v, want %s", l, e1.ID)
	}

	if _, err := b.Publish(open(t, b), []Entry{e2}, author, PublishOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	trace()
	l = open(t, a)
	if got := trace(); !strings.Contains(got, "git fetch") {
		t.Errorf("reopening after another machine pushed ran:\n%s\nwant a fetch", got)
	}
	if l.FetchErr != nil || len(l.IDs) != 2 || !l.IDs[e2.ID] {
		t.Errorf("ledger after the tip moved = %+v, want %s and %s", l, e1.ID, e2.ID)
	}
}

// The fast-import stream is produced as git reads it, whatever the read size.
func TestFastImportStream(t *testing.T) {
	r := newStream("refs/heads/main", "0123abcd", "me", "1+me@users.noreply.github.com", []Entry{e1, e2})
	got, err := io.ReadAll(iotest.OneByteReader(r))
	if err != nil {
		t.Fatal(err)
	}
	want := "commit refs/heads/main\n" +
		"author me <1+me@users.noreply.github.com> 1735835400 +0700\n" +
		"committer me <1+me@users.noreply.github.com> 1735835400 +0700\n" +
		"data 18\ngb/1 aaaaaaaaaaa1\n" +
		"from 0123abcd\n\n" +
		"commit refs/heads/main\n" +
		"author me <1+me@users.noreply.github.com> 1735916400 -0500\n" +
		"committer me <1+me@users.noreply.github.com> 1735916400 -0500\n" +
		"data 18\ngb/1 aaaaaaaaaaa2\n\n"
	if string(got) != want {
		t.Errorf("stream =\n%s\nwant\n%s", got, want)
	}
}
