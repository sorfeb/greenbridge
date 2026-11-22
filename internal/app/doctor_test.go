package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/ident"
	"github.com/sorfeb/greenbridge/internal/source"
)

const (
	testLogin   = "octocat"
	testNoreply = "583231+octocat@users.noreply.github.com"
	testMirror  = "octocat/greenbridge-activity"
)

// homeState is which files the greenbridge home holds.
type homeState int

const (
	homeFull     homeState = iota // config and secret
	homeNoSecret                  // config only
	homeEmpty
)

// doctorHome points GBRIDGE_HOME at a fresh directory. The config is healthy
// until mutate (nil for none) edits it.
func doctorHome(t *testing.T, state homeState, mutate func(*config.Config)) {
	t.Helper()
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	if state == homeEmpty {
		return
	}
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Emails = []string{"me@work.example"}
	cfg.MirrorAuthor = config.MirrorAuthor{Name: testLogin, Email: testNoreply}
	cfg.Mirror.Repo = testMirror
	cfg.Mirror.Remote = "https://github.com/" + testMirror + ".git"
	if mutate != nil {
		mutate(cfg)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if state == homeFull {
		sp, _ := config.SecretPath()
		if err := ident.Save(sp, bytes.Repeat([]byte{7}, ident.SecretSize)); err != nil {
			t.Fatal(err)
		}
	}
}

// healthyGitHub is an account that owns a private, non-fork mirror on main.
func healthyGitHub() *ghx.Fake {
	return &ghx.Fake{
		User:  &ghx.Account{Login: testLogin, NoreplyEmail: testNoreply},
		Repos: map[string]ghx.Repo{testMirror: {Owner: testLogin, Private: true, DefaultBranch: "main"}},
	}
}

func TestDiagnose(t *testing.T) {
	for _, tc := range []struct {
		name string
		home homeState
		cfg  func(*config.Config) // nil: healthy config
		gh   func(*ghx.Fake)      // nil: healthy GitHub
		want map[string]status
	}{
		{
			name: "healthy",
			want: map[string]status{
				"config": statusOK, "secret": statusOK, "emails": statusOK, "remotes": statusOK, "gh": statusOK,
				"author email": statusOK, "mirror": statusOK, "mirror branch": statusOK, "private contributions": statusUnverified,
			},
		},
		{
			name: "gh missing",
			gh:   func(f *ghx.Fake) { f.Err = ghx.ErrNotInstalled },
			want: map[string]status{
				"config": statusOK, "secret": statusOK, "gh": statusFail, "author email": statusSkipped,
				"mirror": statusSkipped, "mirror branch": statusSkipped, "private contributions": statusSkipped,
			},
		},
		{
			name: "gh logged out",
			gh:   func(f *ghx.Fake) { f.User = nil },
			want: map[string]status{"gh": statusFail, "author email": statusSkipped, "mirror": statusSkipped},
		},
		{
			name: "fork",
			gh: func(f *ghx.Fake) {
				f.Repos[testMirror] = ghx.Repo{Owner: testLogin, Fork: true, Private: true, DefaultBranch: "main"}
			},
			want: map[string]status{"mirror": statusFail, "mirror branch": statusSkipped, "private contributions": statusSkipped},
		},
		{
			name: "wrong owner",
			gh: func(f *ghx.Fake) {
				f.Repos[testMirror] = ghx.Repo{Owner: "someone-else", Private: true, DefaultBranch: "main"}
			},
			want: map[string]status{"mirror": statusFail, "mirror branch": statusSkipped},
		},
		{
			name: "wrong branch",
			gh: func(f *ghx.Fake) {
				f.Repos[testMirror] = ghx.Repo{Owner: testLogin, Private: true, DefaultBranch: "trunk"}
			},
			want: map[string]status{"mirror": statusOK, "mirror branch": statusFail, "private contributions": statusUnverified},
		},
		{
			name: "mirror not on GitHub",
			gh:   func(f *ghx.Fake) { delete(f.Repos, testMirror) },
			want: map[string]status{"mirror": statusFail, "mirror branch": statusSkipped},
		},
		{
			name: "public mirror",
			gh:   func(f *ghx.Fake) { f.Repos[testMirror] = ghx.Repo{Owner: testLogin, DefaultBranch: "main"} },
			want: map[string]status{"mirror": statusOK, "private contributions": statusOK},
		},
		{
			name: "unverified email",
			cfg:  func(c *config.Config) { c.MirrorAuthor.Email = "me@example.com" },
			gh:   func(f *ghx.Fake) { f.Addresses = []ghx.Email{{Address: "Me@Example.com", Verified: false}} },
			want: map[string]status{"author email": statusFail},
		},
		{
			name: "verified email",
			cfg:  func(c *config.Config) { c.MirrorAuthor.Email = "me@example.com" },
			gh:   func(f *ghx.Fake) { f.Addresses = []ghx.Email{{Address: "Me@Example.com", Verified: true}} },
			want: map[string]status{"author email": statusOK},
		},
		{
			name: "email scope missing",
			cfg:  func(c *config.Config) { c.MirrorAuthor.Email = "me@example.com" },
			gh:   func(f *ghx.Fake) { f.EmailsErr = errors.New("needs the user scope") },
			want: map[string]status{"author email": statusUnverified},
		},
		{
			name: "local only",
			cfg:  func(c *config.Config) { c.Mirror.Repo, c.Mirror.Remote = "", "" },
			want: map[string]status{
				"gh": statusOK, "mirror": statusWarn, "mirror branch": statusSkipped, "private contributions": statusSkipped,
			},
		},
		{
			name: "source repo missing",
			cfg:  func(c *config.Config) { c.Repos = []string{filepath.Join(t.TempDir(), "gone")} },
			want: map[string]status{"repos": statusFail, "mirror": statusOK},
		},
		{
			name: "scan folder missing",
			cfg:  func(c *config.Config) { c.ScanFolders = []string{filepath.Join(t.TempDir(), "gone")} },
			want: map[string]status{"repos": statusFail, "mirror": statusOK},
		},
		{
			name: "scan folder only",
			cfg:  func(c *config.Config) { c.ScanFolders = []string{t.TempDir()} },
			want: map[string]status{"repos": statusOK},
		},
		{
			name: "no config",
			home: homeEmpty,
			want: map[string]status{
				"config": statusFail, "secret": statusFail, "repos": statusSkipped, "emails": statusSkipped,
				"remotes": statusSkipped, "gh": statusOK,
				"author email": statusSkipped, "mirror": statusSkipped, "mirror branch": statusSkipped,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doctorHome(t, tc.home, tc.cfg)
			gh := healthyGitHub()
			if tc.gh != nil {
				tc.gh(gh)
			}
			got := map[string]result{}
			for _, r := range diagnose(gh) {
				if _, dup := got[r.check]; !dup {
					got[r.check] = r
				}
			}
			for check, want := range tc.want {
				r, ok := got[check]
				switch {
				case !ok:
					t.Errorf("%s: no result; checks never vanish", check)
				case r.status != want:
					t.Errorf("%s: %s %q, want %s", check, r.status, r.msg, want)
				case r.status == statusFail && r.fix == "":
					t.Errorf("%s: failure without a fix", check)
				}
			}
		})
	}
}

// Each failure names its fix once: on the fix line, not also in the message.
func TestDoctorNamesInitOnce(t *testing.T) {
	gh := &ghx.Fake{Err: ghx.ErrNotInstalled}

	doctorHome(t, homeEmpty, nil)
	var out bytes.Buffer
	Doctor(&out, gh)
	if !strings.Contains(out.String(), "no config at") {
		t.Fatalf("missing config not reported:\n%s", out.String())
	}
	if n := strings.Count(out.String(), "run gbridge init"); n != 2 {
		t.Errorf("missing config and secret: %q appears %d times, want once each:\n%s", "run gbridge init", n, out.String())
	}

	doctorHome(t, homeNoSecret, nil)
	out.Reset()
	Doctor(&out, gh)
	if !strings.Contains(out.String(), "no secret at") {
		t.Fatalf("missing secret not reported:\n%s", out.String())
	}
	if n := strings.Count(out.String(), "run gbridge init"); n != 1 {
		t.Errorf("missing secret: %q appears %d times, want 1:\n%s", "run gbridge init", n, out.String())
	}
}

// isolateGitConfig points git at an empty global config holding only
// user.name (none when empty), so the machine's own config can't leak in.
func isolateGitConfig(t *testing.T, userName string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gitconfig")
	body := ""
	if userName != "" {
		body = "[user]\n\tname = " + userName + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", p)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// Commits under an email missing from config never reach the graph. Doctor
// names such an email when its commits carry the user's own author name, and
// stays quiet about coworkers, old history, and tool refs.
func TestDoctorFlagsUnlistedEmails(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "Laptop Me")
	repo := filepath.Join(t.TempDir(), "work")
	gittest.Init(t, repo)
	recent := func(days int) string { return time.Now().AddDate(0, 0, -days).Format(time.RFC3339) }

	gittest.CommitAs(t, repo, "Me Dev", "me@work.example", recent(30))
	gittest.CommitAs(t, repo, "me dev", "Old@Laptop.example", recent(20)) // same name, any case
	gittest.CommitAs(t, repo, "Me Dev", "old@laptop.example", recent(10))
	gittest.CommitAs(t, repo, "Laptop Me", "me@laptop.local", recent(5)) // git's user.name
	gittest.CommitAs(t, repo, "Coworker", "coworker@work.example", recent(5))
	gittest.CommitAs(t, repo, "Me Dev", "ancient@laptop.example", recent(800))
	gittest.CommitOnRef(t, repo, "refs/t3/checkpoints/x/turn/1", "Me Dev", "t3code@users.noreply.github.com", recent(1))

	doctorHome(t, homeFull, func(c *config.Config) { c.Repos = []string{repo} })
	var warns []result
	for _, r := range diagnose(healthyGitHub()) {
		if r.check == "emails" {
			warns = append(warns, r)
		}
	}
	want := []struct {
		email string
		count string
	}{{"old@laptop.example", "2 commits in"}, {"me@laptop.local", "1 commit in"}}
	if len(warns) != len(want) {
		t.Fatalf("emails results = %+v, want one warning each for %+v", warns, want)
	}
	for i, w := range want {
		r := warns[i]
		if r.status != statusWarn || !strings.Contains(r.msg, w.email) || !strings.Contains(r.msg, w.count) ||
			r.fix != "gbridge add email "+w.email {
			t.Errorf("result %d = %+v, want a warning naming %s with %s and its fix", i, r, w.email, w.count)
		}
	}

	var out bytes.Buffer
	Doctor(&out, healthyGitHub())
	for _, leak := range []string{"coworker", "Coworker", "ancient", "t3code", "Me Dev", gittest.Message} {
		if strings.Contains(out.String(), leak) {
			t.Errorf("doctor output contains %q:\n%s", leak, out.String())
		}
	}
	if !strings.Contains(out.String(), "fix: gbridge add email old@laptop.example") {
		t.Errorf("doctor output lacks the fix line:\n%s", out.String())
	}
}

// A commit made under the user's name with a coworker's email (a shared
// machine, a pairing setup) must not lead to adding that email: every commit
// the coworker made under it would then be mirrored as the user's.
func TestDoctorNeverSuggestsSharedEmail(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "")
	repo := filepath.Join(t.TempDir(), "work")
	gittest.Init(t, repo)
	recent := func(days int) string { return time.Now().AddDate(0, 0, -days).Format(time.RFC3339) }

	gittest.CommitAs(t, repo, "Me Dev", "me@work.example", recent(30))
	gittest.CommitAs(t, repo, "Me Dev", "pair@work.example", recent(20))
	gittest.CommitAs(t, repo, "Pair Partner", "pair@work.example", recent(15))
	gittest.CommitAs(t, repo, "Pair Partner", "pair@work.example", recent(10))

	doctorHome(t, homeFull, func(c *config.Config) { c.Repos = []string{repo} })
	var got []result
	for _, r := range diagnose(healthyGitHub()) {
		if r.check == "emails" {
			got = append(got, r)
		}
	}
	if len(got) != 1 {
		t.Fatalf("emails results = %+v, want exactly one", got)
	}
	r := got[0]
	if r.status != statusOK || r.fix != "" {
		t.Errorf("result = %+v, want ok with no fix: nothing safe to add", r)
	}
	for _, want := range []string{"1 commit", "pair@work.example", "2 commits under other author names"} {
		if !strings.Contains(r.msg, want) {
			t.Errorf("msg = %q, want it to contain %q", r.msg, want)
		}
	}

	var out bytes.Buffer
	Doctor(&out, healthyGitHub())
	for _, leak := range []string{"gbridge add email pair@work.example", "Pair Partner"} {
		if strings.Contains(out.String(), leak) {
			t.Errorf("doctor output contains %q:\n%s", leak, out.String())
		}
	}
}

// Commits on a remote reach sync only through git fetch. Doctor compares
// each tracked remote with the clone and names the fetch, or the broken
// remote, to fix. How long ago the clone fetched does not matter.
func TestDoctorChecksRemotes(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "")
	dir := t.TempDir()
	up, behind, current := filepath.Join(dir, "up"), filepath.Join(dir, "behind"), filepath.Join(dir, "current")
	gittest.Init(t, up)
	gittest.Commit(t, up, "me@work.example", "2025-03-01T10:00:00Z")
	gittest.Git(t, "", nil, "clone", "-q", up, behind)
	gittest.Git(t, "", nil, "clone", "-q", up, current)
	gittest.Commit(t, up, "me@work.example", "2025-03-02T10:00:00Z")
	gittest.Git(t, current, nil, "fetch", "-q")
	gone := filepath.Join(dir, "gone")
	gittest.Git(t, current, nil, "remote", "add", "dev-vm", gone)
	gittest.Git(t, current, nil, "config", "branch.main.remote", "dev-vm")
	gittest.Git(t, behind, nil, "remote", "add", "leftover", gone)
	doctorHome(t, homeFull, func(c *config.Config) { c.Repos = []string{behind, current} })

	var got []result
	for _, r := range diagnose(healthyGitHub()) {
		if r.check == "remotes" {
			got = append(got, r)
		}
	}
	want := []result{
		{"remotes", statusWarn, "1 branch on origin has commits you haven't fetched: " + behind, "git -C " + behind + " fetch origin"},
		{"remotes", statusWarn, "can't reach dev-vm: " + current, "git -C " + current + " remote remove dev-vm, or fix its URL: git -C " + current + " remote set-url dev-vm <url>"},
		{"remotes", statusOK, "1 tracked remote checked, nothing left to fetch", ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("remotes results:\n got  %+v\n want %+v", got, want)
	}

	var out bytes.Buffer
	Doctor(&out, healthyGitHub())
	if !strings.Contains(out.String(), "fix: git -C "+behind+" fetch origin") {
		t.Errorf("doctor output lacks the fix line:\n%s", out.String())
	}
}

// A forge that answers after the deadline is slow, not broken: its remotes
// share one unverified line and never a fix that removes them.
func TestCheckRemotesSlow(t *testing.T) {
	late := fmt.Errorf("listing origin: %w", context.DeadlineExceeded)
	f := &facts{cfg: &config.Config{}, repos: []repoFacts{
		{path: "a", remotes: []source.Remote{{Name: "origin", Err: late}, {Name: "vm"}}},
		{path: "b", remotes: []source.Remote{{Name: "origin", Err: late}}},
	}}
	want := []result{
		{"remotes", statusOK, "1 tracked remote checked, nothing left to fetch", ""},
		{"remotes", statusUnverified, "2 tracked remotes gave no answer within 15s, not checked", "run gbridge doctor again, or git fetch in those repos"},
	}
	if got := checkRemotes(f); !slices.Equal(got, want) {
		t.Errorf("checkRemotes:\n got  %+v\n want %+v", got, want)
	}
}

func TestDoctorPrintsSkipped(t *testing.T) {
	doctorHome(t, homeFull, nil)
	var out bytes.Buffer
	fails := Doctor(&out, &ghx.Fake{Err: ghx.ErrNotInstalled})
	if fails != 1 {
		t.Errorf("Doctor() = %d failures, want 1 (gh only; skipped checks are not failures)\n%s", fails, out.String())
	}
	for _, line := range []string{"[FAIL]       gh not found", "[skipped]    author email", "[skipped]    mirror"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output lacks %q:\n%s", line, out.String())
		}
	}
}
