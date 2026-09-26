// Package mirror manages the activity repo. It keeps no state file: the set
// of synced IDs is read back from commit subjects ("gb/1 <id>").
package mirror

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/gitx"
)

// Mirror is a local bare clone of the activity repo.
type Mirror struct {
	Path   string
	Remote string // empty for local only
	Branch string
}

// Entry is one commit to write.
type Entry struct {
	ID   string
	Time time.Time
}

// Author is the identity on mirror commits.
type Author struct {
	Name  string
	Email string
}

// Ensure creates the bare clone if missing and points origin at Remote.
func (m Mirror) Ensure() error {
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

// Fetch returns the current tip of the mirror branch, fetching from origin
// when there is a remote. An empty string means the branch has no commits.
func (m Mirror) Fetch() (string, error) {
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
	if _, err := gitx.Git(m.Path, "fetch", "-q", "origin", "+refs/heads/"+m.Branch+":"+tracking); err != nil {
		return "", err
	}
	return m.rev(tracking), nil
}

// Ledger returns the IDs already present at tip.
func (m Mirror) Ledger(tip string) (map[string]bool, error) {
	ids := map[string]bool{}
	if tip == "" {
		return ids, nil
	}
	out, err := gitx.Git(m.Path, "log", "--format=%s", tip)
	if err != nil {
		return nil, err
	}
	for _, s := range strings.Split(out, "\n") {
		if id := ParseSubject(s); id != "" {
			ids[id] = true
		}
	}
	return ids, nil
}

// ParseSubject extracts the ID from "gb/<version> [fields...] <id>".
// Unknown future versions are accepted; non-gb commits return "".
func ParseSubject(s string) string {
	f := strings.Fields(s)
	if len(f) < 2 || !strings.HasPrefix(f[0], "gb/") {
		return ""
	}
	if _, err := strconv.Atoi(f[0][3:]); err != nil {
		return ""
	}
	return f[len(f)-1]
}

// Emit writes one empty commit per entry on top of tip and moves the local
// branch there. Any unpushed local commits are discarded: they are derived
// data and get recomputed.
func (m Mirror) Emit(tip string, entries []Entry, a Author) (string, error) {
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
	name, email := clean(a.Name), clean(a.Email)
	var b strings.Builder
	for i, e := range entries {
		date := fmt.Sprintf("%d %s", e.Time.Unix(), e.Time.Format("-0700"))
		msg := "gb/1 " + e.ID + "\n"
		fmt.Fprintf(&b, "commit %s\nauthor %s <%s> %s\ncommitter %s <%s> %s\ndata %d\n%s", ref, name, email, date, name, email, date, len(msg), msg)
		if i == 0 && tip != "" {
			fmt.Fprintf(&b, "from %s\n", tip)
		}
		b.WriteString("\n")
	}
	if _, err := gitx.Run(m.Path, nil, strings.NewReader(b.String()), "fast-import", "--quiet"); err != nil {
		return "", err
	}
	return m.rev(ref), nil
}

// Push sends the local branch to origin. It fails without forcing if the
// remote moved; the caller refetches and retries.
func (m Mirror) Push() error {
	ref := "refs/heads/" + m.Branch
	_, err := gitx.Git(m.Path, "push", "-q", "origin", ref+":"+ref)
	return err
}

// Count returns the number of commits reachable from tip.
func (m Mirror) Count(tip string) int {
	if tip == "" {
		return 0
	}
	out, err := gitx.Git(m.Path, "rev-list", "--count", tip)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(out)
	return n
}

func (m Mirror) rev(ref string) string {
	out, err := gitx.Git(m.Path, "rev-parse", "-q", "--verify", ref+"^{commit}")
	if err != nil {
		return ""
	}
	return out
}

func clean(s string) string {
	return strings.NewReplacer("<", "", ">", "", "\n", " ").Replace(s)
}
