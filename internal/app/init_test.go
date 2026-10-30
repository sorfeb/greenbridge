package app

import (
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
)

func TestInitCreatesMirrorOnGitHub(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	gh := &ghx.Fake{User: &ghx.Account{Login: testLogin, NoreplyEmail: testNoreply}}

	o := InitOptions{Emails: []string{"me@work.example"}, MirrorName: "greenbridge-activity", Yes: true}
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
	if cfg.Mirror.Repo != testMirror || cfg.Author.Email != testNoreply || cfg.Author.Name != testLogin {
		t.Errorf("config mirror %q author %+v", cfg.Mirror.Repo, cfg.Author)
	}
}

func TestInitWithoutGHNeedsLocalOnly(t *testing.T) {
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	o := InitOptions{Emails: []string{"me@work.example"}, AuthorEmail: "me@example.com", Yes: true}
	err := Init(o, &ghx.Fake{Err: ghx.ErrNotInstalled}, strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--local-only") {
		t.Errorf("Init() without gh = %v, want a hint to pass --local-only", err)
	}
}
