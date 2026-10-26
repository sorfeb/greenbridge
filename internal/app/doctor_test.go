package app

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/ident"
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
	cfg.Author = config.Author{Name: testLogin, Email: testNoreply}
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
				"config": statusOK, "secret": statusOK, "gh": statusOK, "author email": statusOK,
				"mirror": statusOK, "mirror branch": statusOK, "private contributions": statusUnverified,
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
			cfg:  func(c *config.Config) { c.Author.Email = "me@example.com" },
			gh:   func(f *ghx.Fake) { f.Addresses = []ghx.Email{{Address: "Me@Example.com", Verified: false}} },
			want: map[string]status{"author email": statusFail},
		},
		{
			name: "verified email",
			cfg:  func(c *config.Config) { c.Author.Email = "me@example.com" },
			gh:   func(f *ghx.Fake) { f.Addresses = []ghx.Email{{Address: "Me@Example.com", Verified: true}} },
			want: map[string]status{"author email": statusOK},
		},
		{
			name: "email scope missing",
			cfg:  func(c *config.Config) { c.Author.Email = "me@example.com" },
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
			name: "no config",
			home: homeEmpty,
			want: map[string]status{
				"config": statusFail, "secret": statusFail, "repos": statusSkipped, "gh": statusOK,
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
