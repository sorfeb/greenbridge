package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/ident"
)

const testWorkEmail = "me@work.example"

func loggedIn() *ghx.Fake {
	return &ghx.Fake{User: &ghx.Account{Login: testLogin, NoreplyEmail: testNoreply}}
}

// script is stdin answering one prompt per line.
func script(answers ...string) io.Reader {
	return strings.NewReader(strings.Join(answers, "\n") + "\n")
}

// readSecret returns the secret init saved, failing the test when there is none.
func readSecret(t *testing.T) []byte {
	t.Helper()
	sp, _ := config.SecretPath()
	s, err := ident.Load(sp)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// assertNothingWritten fails when init left a config, a secret, a local
// mirror, or a repo on GitHub.
func assertNothingWritten(t *testing.T, gh *ghx.Fake) {
	t.Helper()
	cp, _ := config.Path()
	sp, _ := config.SecretPath()
	cfg, _ := config.Default()
	for _, p := range []string{cp, sp, cfg.Mirror.Path} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left behind", filepath.Base(p))
		}
	}
	if len(gh.Repos) > 0 {
		t.Errorf("repos left on GitHub: %v", gh.Repos)
	}
}

func TestInitCreatesMirrorOnGitHub(t *testing.T) {
	gittest.Need(t)
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	gh := loggedIn()

	o := InitOptions{Emails: []string{testWorkEmail}, MirrorName: "greenbridge-activity", Yes: true}
	if err := Init(o, gh, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if r, err := gh.Repo(testMirror); err != nil || !r.Private {
		t.Errorf("mirror on GitHub: %+v, %v; want a private repo", r, err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mirror.Repo != testMirror || cfg.MirrorAuthor.Email != testNoreply || cfg.MirrorAuthor.Name != testLogin {
		t.Errorf("config mirror %q author %+v", cfg.Mirror.Repo, cfg.MirrorAuthor)
	}
}

func TestInitInteractive(t *testing.T) {
	gittest.Need(t)
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	work := t.TempDir()
	mine, theirs, hosted := filepath.Join(work, "mine"), filepath.Join(work, "theirs"), filepath.Join(work, "hosted")
	for _, r := range []string{mine, theirs, hosted} {
		gittest.Init(t, r)
	}
	gittest.Commit(t, mine, testWorkEmail, "2025-01-02T10:00:00Z")
	gittest.Commit(t, theirs, "someone@work.example", "2025-01-02T10:00:00Z")
	gittest.Commit(t, hosted, testWorkEmail, "2025-01-02T10:00:00Z")
	gittest.Git(t, hosted, nil, "remote", "add", "origin", "https://github.com/acme/hosted.git")
	gh := loggedIn()

	var out bytes.Buffer
	in := script(
		"",            // Continue? (y/n) [y]
		testWorkEmail, // Work emails
		work,          // Folders to scan
		"y",           // Use these 1 repos?
		"",            // Mirror repo name [greenbridge-activity]
		"y",           // Create private repo?
	)
	if err := Init(InitOptions{}, gh, in, &out); err != nil {
		t.Fatalf("Init() = %v\n%s", err, out.String())
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// The folder is kept, so later clones are found; what it holds now is
	// recorded as seen, since init already showed it.
	if !slices.Equal(cfg.Emails, []string{testWorkEmail}) || len(cfg.Repos) != 0 || !slices.Equal(cfg.ScanFolders, []string{work}) {
		t.Errorf("config emails %v repos %v scan %v, want [%s] [] [%s]", cfg.Emails, cfg.Repos, cfg.ScanFolders, testWorkEmail, work)
	}
	if seen, err := config.LoadSeen(); err != nil || len(seen) != 1 || !seen[mine] {
		t.Errorf("seen = %v, %v; want %s only", seen, err, mine)
	}
	if !cfg.NoticeAccepted || cfg.Mirror.Repo != testMirror || cfg.Mirror.Remote != "https://github.com/"+testMirror+".git" {
		t.Errorf("config notice %v mirror %+v", cfg.NoticeAccepted, cfg.Mirror)
	}
	if r, err := gh.Repo(testMirror); err != nil || !r.Private {
		t.Errorf("mirror on GitHub: %+v, %v; want a private repo", r, err)
	}
	if !strings.Contains(out.String(), ident.Encode(readSecret(t))) {
		t.Errorf("output does not show the recovery phrase of the saved secret:\n%s", out.String())
	}
	if remote := gittest.Git(t, cfg.Mirror.Path, nil, "remote", "get-url", "origin"); remote != cfg.Mirror.Remote {
		t.Errorf("local mirror origin = %q, want %q", remote, cfg.Mirror.Remote)
	}
}

// A failed init must leave nothing to clean up: no secret that would
// otherwise be kept on the next run, and no repo on GitHub.
func TestInitFailureLeavesNothingBehind(t *testing.T) {
	tests := []struct {
		name string
		o    InitOptions
		gh   *ghx.Fake
		in   io.Reader
		want string // substring of the error
	}{
		{
			name: "mirror name GitHub rejects",
			gh:   loggedIn(),
			in:   script("", testWorkEmail, "", "my mirror", "y"),
			want: "mirror.repo",
		},
		{
			name: "author email without @",
			o:    InitOptions{Emails: []string{testWorkEmail}, AuthorEmail: "octocat", Yes: true},
			gh:   loggedIn(),
			want: "author.email",
		},
		{
			name: "mirror creation declined",
			gh:   loggedIn(),
			in:   script("", testWorkEmail, "", "", "n"),
			want: "aborted",
		},
		{
			name: "gh logged out without --local-only",
			o:    InitOptions{Emails: []string{testWorkEmail}, AuthorEmail: "me@example.com", Yes: true},
			gh:   &ghx.Fake{Err: ghx.ErrNotInstalled},
			want: "--local-only",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gittest.Need(t)
			t.Setenv("GBRIDGE_HOME", t.TempDir())
			in := tt.in
			if in == nil {
				in = strings.NewReader("")
			}
			err := Init(tt.o, tt.gh, in, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Init() = %v, want an error containing %q", err, tt.want)
			}
			assertNothingWritten(t, tt.gh)
		})
	}
}

func TestInitImportsSecretForLocalMirror(t *testing.T) {
	gittest.Need(t)
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	want := bytes.Repeat([]byte{7}, ident.SecretSize)

	o := InitOptions{
		Emails: []string{testWorkEmail}, AuthorEmail: "me@example.com", LocalOnly: true,
		Secret: ident.Encode(want), Yes: true,
	}
	if err := Init(o, &ghx.Fake{Err: ghx.ErrNotInstalled}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := readSecret(t); !bytes.Equal(got, want) {
		t.Errorf("secret = %x, want the imported %x", got, want)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Mirror.LocalOnly() || cfg.Mirror.Repo != "" || cfg.MirrorAuthor.Name != "greenbridge" {
		t.Errorf("config mirror %+v author %+v, want local only by greenbridge", cfg.Mirror, cfg.MirrorAuthor)
	}
}

// A new secret changes every mirror ID, so the next sync would double count.
func TestInitKeepsDifferentSecretWithoutForce(t *testing.T) {
	gittest.Need(t)
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	old := bytes.Repeat([]byte{7}, ident.SecretSize)
	sp, _ := config.SecretPath()
	if err := ident.Save(sp, old); err != nil {
		t.Fatal(err)
	}
	gh := loggedIn()

	o := InitOptions{Emails: []string{testWorkEmail}, Secret: ident.Encode(bytes.Repeat([]byte{9}, ident.SecretSize)), Yes: true}
	if err := Init(o, gh, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("Init() = %v, want a hint to pass --force", err)
	}
	if got := readSecret(t); !bytes.Equal(got, old) {
		t.Errorf("secret replaced: %x, want %x", got, old)
	}
	if len(gh.Repos) > 0 {
		t.Errorf("repos created on GitHub: %v", gh.Repos)
	}
}
