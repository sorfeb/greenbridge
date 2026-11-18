package source

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sorfeb/greenbridge/internal/gittest"
)

// upstreamClone makes a repo with one commit on main and a clone of it, and
// returns both paths.
func upstreamClone(t *testing.T) (up, clone string) {
	t.Helper()
	dir := t.TempDir()
	up, clone = filepath.Join(dir, "up"), filepath.Join(dir, "clone")
	gittest.Init(t, up)
	gittest.Commit(t, up, "me@work.example", "2025-03-01T10:00:00Z")
	gittest.Git(t, "", nil, "clone", "-q", up, clone)
	return up, clone
}

func checkRemotes(t *testing.T, repo string) map[string]Remote {
	t.Helper()
	rs, err := CheckRemotes(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Remote{}
	for _, r := range rs {
		got[r.Name] = r
	}
	return got
}

// A remote is current when every branch tip it lists is already in the
// clone, however long ago the clone last fetched.
func TestCheckRemotesCurrent(t *testing.T) {
	gittest.Need(t)
	_, clone := upstreamClone(t)
	got := checkRemotes(t, clone)
	if r := got["origin"]; r.Unfetched != 0 || r.Err != nil {
		t.Errorf("origin = %+v, want current", r)
	}
}

// Unfetched counts remote branches whose tip commit the clone lacks. A new
// branch at a commit the clone already holds hides nothing from sync.
func TestCheckRemotesUnfetched(t *testing.T) {
	gittest.Need(t)
	up, clone := upstreamClone(t)
	gittest.Git(t, up, nil, "branch", "same-commit")
	gittest.Commit(t, up, "me@work.example", "2025-03-02T10:00:00Z")
	gittest.Git(t, up, nil, "checkout", "-q", "-b", "feature")
	gittest.Commit(t, up, "me@work.example", "2025-03-03T10:00:00Z")

	if r := checkRemotes(t, clone)["origin"]; r.Unfetched != 2 || r.Err != nil {
		t.Errorf("origin = %+v, want 2 unfetched branches (main, feature)", r)
	}
	gittest.Git(t, clone, nil, "fetch", "-q", "origin")
	if r := checkRemotes(t, clone)["origin"]; r.Unfetched != 0 || r.Err != nil {
		t.Errorf("origin after fetch = %+v, want current", r)
	}
}

// A single-branch clone tracks one branch; others on the remote are not
// missing, since git fetch would never bring them.
func TestCheckRemotesFollowsRefspec(t *testing.T) {
	gittest.Need(t)
	up, clone := upstreamClone(t)
	gittest.Git(t, clone, nil, "config", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")
	gittest.Git(t, up, nil, "checkout", "-q", "-b", "feature")
	gittest.Commit(t, up, "me@work.example", "2025-03-03T10:00:00Z")

	if r := checkRemotes(t, clone)["origin"]; r.Unfetched != 0 || r.Err != nil {
		t.Errorf("origin = %+v, want current: feature is outside the refspec", r)
	}
}

// Only origin and remotes a local branch tracks are checked. A remote whose
// path is gone reports an error instead of a count.
func TestCheckRemotesTrackedOnly(t *testing.T) {
	gittest.Need(t)
	_, clone := upstreamClone(t)
	gone := filepath.Join(t.TempDir(), "gone")
	gittest.Git(t, clone, nil, "remote", "add", "dev-vm", gone)
	gittest.Git(t, clone, nil, "remote", "add", "leftover", gone)
	gittest.Git(t, clone, nil, "branch", "vm-work")
	gittest.Git(t, clone, nil, "config", "branch.vm-work.remote", "dev-vm")
	gittest.Git(t, clone, nil, "config", "branch.main.pushRemote", "leftover")
	gittest.Git(t, clone, nil, "branch", "local-only")
	gittest.Git(t, clone, nil, "config", "branch.local-only.remote", ".")

	rs, err := CheckRemotes(context.Background(), clone)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range rs {
		names = append(names, r.Name)
	}
	if !slices.Equal(names, []string{"origin", "dev-vm"}) {
		t.Fatalf("checked %v, want [origin dev-vm]", names)
	}
	if rs[0].Err != nil {
		t.Errorf("origin = %+v, want reachable", rs[0])
	}
	if rs[1].Err == nil {
		t.Errorf("dev-vm = %+v, want an error for a missing path", rs[1])
	}
}

// A context that ends first stops ls-remote and names the timeout.
func TestCheckRemotesTimeout(t *testing.T) {
	gittest.Need(t)
	_, clone := upstreamClone(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rs, err := CheckRemotes(ctx, clone)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || !errors.Is(rs[0].Err, context.Canceled) {
		t.Errorf("remotes = %+v, want origin with a context error", rs)
	}
}

func TestNoPromptEnv(t *testing.T) {
	for _, tt := range []struct {
		name    string
		env     map[string]string
		sshConf bool
		want    []string
	}{
		{"plain", nil, false, []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_SSH_COMMAND=ssh -o BatchMode=yes"}},
		{"own GIT_SSH_COMMAND", map[string]string{"GIT_SSH_COMMAND": "plink"}, false, []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}},
		{"own GIT_SSH", map[string]string{"GIT_SSH": "plink.exe"}, false, []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}},
		{"core.sshCommand", nil, true, []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := noPromptEnv(func(k string) string { return tt.env[k] }, tt.sshConf)
			if !slices.Equal(got, tt.want) {
				t.Errorf("noPromptEnv = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMapRef(t *testing.T) {
	specs := parseRefspecs([]string{
		"+refs/heads/*:refs/remotes/origin/*",
		"^refs/heads/tmp/*",
		"refs/tags/v1:refs/tags/v1",
		"refs/heads/only-fetch-head",
	})
	for _, tt := range []struct {
		ref, want string
		ok        bool
	}{
		{"refs/heads/main", "refs/remotes/origin/main", true},
		{"refs/heads/feat/x", "refs/remotes/origin/feat/x", true},
		{"refs/heads/tmp/scratch", "", false},
		{"refs/tags/v1", "refs/tags/v1", true},
		{"refs/tags/v2", "", false},
		{"refs/heads/only-fetch-head", "refs/remotes/origin/only-fetch-head", true},
		{"refs/pull/1/head", "", false},
	} {
		got, ok := mapRef(specs, tt.ref)
		if got != tt.want || ok != tt.ok {
			t.Errorf("mapRef(%q) = %q, %v; want %q, %v", tt.ref, got, ok, tt.want, tt.ok)
		}
	}
}
