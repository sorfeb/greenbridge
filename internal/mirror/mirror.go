// Package mirror owns the append-and-publish protocol: open the mirror clone
// and read its ledger, then append one mirror commit per missing entry and
// push, retrying when another machine pushed first. It keeps no state file:
// the ledger is read back from mirror commit subjects ("gb/1 <id>"), a
// format private to this package.
package mirror

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/gitx"
)

// Clone is the mirror clone: the local bare clone of the mirror repo.
type Clone struct {
	Path   string
	Remote string // empty for local only
	Branch string
}

// Entry is one mirror ID and the time to write for it: one mirror commit.
type Entry struct {
	ID   string
	Time time.Time
}

// Author is the mirror author, the identity on every mirror commit.
type Author struct {
	Name  string
	Email string
}

// Ensure creates the bare clone if missing and points origin at Remote.
func (m Clone) Ensure() error {
	if _, err := os.Stat(filepath.Join(m.Path, "HEAD")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(m.Path), 0o700); err != nil {
			return err
		}
		if _, err := gitx.Git("", "init", "-q", "--bare", "-b", m.Branch, m.Path); err != nil {
			return err
		}
	}
	if m.Remote == "" {
		return nil
	}
	if cur, err := gitx.Git(m.Path, "remote", "get-url", "origin"); err != nil {
		_, err = gitx.Git(m.Path, "remote", "add", "origin", m.Remote)
		return err
	} else if cur != m.Remote {
		_, err = gitx.Git(m.Path, "remote", "set-url", "origin", m.Remote)
		return err
	}
	return nil
}

// Ledger is the set of mirror IDs in the mirror repo, read at one tip.
type Ledger struct {
	IDs      map[string]bool
	FetchErr error // fetch failed: IDs are from the last successful fetch

	tip string // mirror commit new entries build on; empty for none
}

// Open ensures the mirror clone exists, fetches the mirror repo, and reads
// the ledger. A failed fetch is recorded in FetchErr, not returned: a preview
// still works offline, and Publish refuses to write from it.
func (m Clone) Open() (*Ledger, error) {
	if err := m.Ensure(); err != nil {
		return nil, err
	}
	l := &Ledger{}
	return l, m.read(l)
}

// read refills l from the mirror repo's current tip, falling back to the
// last fetched tip when the fetch fails.
func (m Clone) read(l *Ledger) error {
	tip, err := m.fetch()
	l.FetchErr = err
	if err != nil {
		tip = m.last()
	}
	ids, err := m.ids(tip)
	if err != nil {
		return err
	}
	l.IDs, l.tip = ids, tip
	return nil
}

// Missing returns the entries whose mirror IDs are not in the ledger, in
// their order.
func (l *Ledger) Missing(desired []Entry) []Entry {
	var out []Entry
	for _, e := range desired {
		if !l.IDs[e.ID] {
			out = append(out, e)
		}
	}
	return out
}

// Orphans counts the mirror IDs in the ledger that no desired entry
// produces: work from another machine sharing the mirror repo, from a source
// repo or email no longer configured, or history rewritten in sha identity
// mode. Publish never removes them.
func (l *Ledger) Orphans(desired []Entry) int {
	want := make(map[string]bool, len(desired))
	for _, e := range desired {
		want[e.ID] = true
	}
	n := 0
	for id := range l.IDs {
		if !want[id] {
			n++
		}
	}
	return n
}

// PublishOptions controls how Publish writes.
type PublishOptions struct {
	DryRun bool // compute only, write nothing
	NoPush bool // write the mirror clone, skip the push
}

// Published reports what Publish did.
type Published struct {
	Ledger int  // mirror IDs already in the mirror repo
	Added  int  // mirror commits written, or that a dry run would write
	Pushed bool // the mirror repo has them
}

// attempts is how many times Publish tries to push before giving up.
const attempts = 3

// Publish appends one mirror commit per desired entry missing from l, on top
// of the tip l was read at, and pushes it. It refuses to write after a
// failed fetch: entries built on a stale tip either duplicate what another
// machine pushed or get rejected. A rejected push is never forced: Publish
// refetches into l, recomputes what is missing, and tries again. On success
// l holds every published ID. log receives one line per rejected push and
// may be nil.
func (m Clone) Publish(l *Ledger, desired []Entry, a Author, opt PublishOptions, log io.Writer) (Published, error) {
	if log == nil {
		log = io.Discard
	}
	for attempt := range attempts {
		if attempt > 0 {
			if err := m.read(l); err != nil {
				return Published{}, err
			}
		}
		if l.FetchErr != nil {
			return Published{}, fmt.Errorf("mirror fetch failed, nothing written: %w", l.FetchErr)
		}
		missing := l.Missing(desired)
		res := Published{Ledger: len(l.IDs), Added: len(missing)}
		if len(missing) == 0 || opt.DryRun {
			return res, nil
		}
		tip, err := m.emit(l.tip, missing, a)
		if err != nil {
			return Published{}, err
		}
		if m.Remote != "" && !opt.NoPush {
			if err := m.push(); err != nil {
				fmt.Fprintf(log, "push rejected, refetching (attempt %d): %v\n", attempt+1, err)
				continue
			}
			res.Pushed = true
		}
		for _, e := range missing {
			l.IDs[e.ID] = true
		}
		l.tip = tip
		return res, nil
	}
	return Published{}, fmt.Errorf("push kept failing after %d attempts; run gbridge doctor", attempts)
}

// fetch returns the current tip of the mirror branch. With a remote it asks
// origin for the tip and fetches only when the mirror clone lacks it. An
// empty string means the branch has no commits.
func (m Clone) fetch() (string, error) {
	if m.Remote == "" {
		return m.rev("refs/heads/" + m.Branch), nil
	}
	heads, err := gitx.Git(m.Path, "ls-remote", "--heads", "origin", m.Branch)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(heads) == "" {
		return "", nil
	}
	tracking := "refs/remotes/origin/" + m.Branch
	// The mirror clone already has the remote's tip: a fetch would bring
	// nothing.
	if tip := m.rev(tracking); tip != "" && tip == remoteTip(heads, "refs/heads/"+m.Branch) {
		return tip, nil
	}
	if _, err := gitx.Git(m.Path, "fetch", "-q", "origin", "+refs/heads/"+m.Branch+":"+tracking); err != nil {
		return "", err
	}
	return m.rev(tracking), nil
}

// remoteTip returns the object ID git ls-remote lists for ref, or "" when
// ref is not listed. ls-remote matches its pattern against the end of every
// ref name, so a branch named x/main also matches main.
func remoteTip(lsRemote, ref string) string {
	for line := range strings.SplitSeq(lsRemote, "\n") {
		if id, name, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok && name == ref {
			return id
		}
	}
	return ""
}

// last returns the tip from the most recent successful fetch without
// contacting origin. An empty string means none is known.
func (m Clone) last() string {
	if m.Remote == "" {
		return m.rev("refs/heads/" + m.Branch)
	}
	return m.rev("refs/remotes/origin/" + m.Branch)
}

// ids returns the mirror IDs already present at tip.
func (m Clone) ids(tip string) (map[string]bool, error) {
	ids := map[string]bool{}
	if tip == "" {
		return ids, nil
	}
	for s, err := range gitx.Lines(m.Path, "log", "--format=%s", tip) {
		if err != nil {
			return nil, err
		}
		if id := parseSubject(s); id != "" {
			ids[id] = true
		}
	}
	return ids, nil
}

// parseSubject extracts the ID from "gb/<version> [fields...] <id>".
// Unknown future versions are accepted; non-gb commits return "".
func parseSubject(s string) string {
	f := strings.Fields(s)
	if len(f) < 2 || !strings.HasPrefix(f[0], "gb/") {
		return ""
	}
	if _, err := strconv.Atoi(f[0][3:]); err != nil {
		return ""
	}
	return f[len(f)-1]
}

// emit writes one empty mirror commit per entry on top of tip and moves the
// local branch there. Any unpushed local commits are discarded: they are derived
// data and get recomputed.
func (m Clone) emit(tip string, entries []Entry, a Author) (string, error) {
	ref := "refs/heads/" + m.Branch
	if tip != "" {
		if _, err := gitx.Git(m.Path, "update-ref", ref, tip); err != nil {
			return "", err
		}
	} else if m.rev(ref) != "" {
		if _, err := gitx.Git(m.Path, "update-ref", "-d", ref); err != nil {
			return "", err
		}
	}
	stream := newStream(ref, tip, clean(a.Name), clean(a.Email), entries)
	if _, err := gitx.Run(m.Path, nil, stream, "fast-import", "--quiet"); err != nil {
		return "", err
	}
	return m.rev(ref), nil
}

// stream is the git fast-import input for one mirror commit per entry, on
// top of tip when there is one. Each commit's text is made as git reads it,
// so a large batch never sits in memory twice.
type stream struct {
	ref, tip, name, email string
	entries               []Entry
	next                  int // index of the next entry to write
	buf                   bytes.Buffer
}

func newStream(ref, tip, name, email string, entries []Entry) *stream {
	return &stream{ref: ref, tip: tip, name: name, email: email, entries: entries}
}

func (s *stream) Read(p []byte) (int, error) {
	for s.buf.Len() == 0 {
		if s.next == len(s.entries) {
			return 0, io.EOF
		}
		e := s.entries[s.next]
		date := fmt.Sprintf("%d %s", e.Time.Unix(), e.Time.Format("-0700"))
		msg := "gb/1 " + e.ID + "\n"
		fmt.Fprintf(&s.buf, "commit %s\nauthor %s <%s> %s\ncommitter %s <%s> %s\ndata %d\n%s",
			s.ref, s.name, s.email, date, s.name, s.email, date, len(msg), msg)
		if s.next == 0 && s.tip != "" {
			fmt.Fprintf(&s.buf, "from %s\n", s.tip)
		}
		s.buf.WriteString("\n")
		s.next++
	}
	return s.buf.Read(p)
}

// push sends the local branch to origin. It fails without forcing if the
// remote moved.
func (m Clone) push() error {
	ref := "refs/heads/" + m.Branch
	_, err := gitx.Git(m.Path, "push", "-q", "origin", ref+":"+ref)
	return err
}

func (m Clone) rev(ref string) string {
	out, err := gitx.Git(m.Path, "rev-parse", "-q", "--verify", ref+"^{commit}")
	if err != nil {
		return ""
	}
	return out
}

func clean(s string) string {
	return strings.NewReplacer("<", "", ">", "", "\n", " ").Replace(s)
}
