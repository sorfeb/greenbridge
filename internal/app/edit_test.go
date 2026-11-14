package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/ident"
)

// homeFiles reads the config and secret files byte for byte, so a test can
// prove an edit left them alone.
func homeFiles(t *testing.T) (cfg, secret []byte) {
	t.Helper()
	cp, _ := config.Path()
	sp, _ := config.SecretPath()
	cfg, err := os.ReadFile(cp)
	if err != nil {
		t.Fatal(err)
	}
	secret, err = os.ReadFile(sp)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, secret
}

func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Adding an email is the fix for commits made under an old address. It must
// not touch the secret or the mirror, or every mirror ID changes.
func TestAddEmailKeepsSecretAndMirror(t *testing.T) {
	doctorHome(t, homeFull, nil)
	before := loadConfig(t)
	_, secret := homeFiles(t)

	var out bytes.Buffer
	if err := Add("email", []string{"old@laptop.example", " ME@work.example "}, &out); err != nil {
		t.Fatalf("Add() = %v\n%s", err, out.String())
	}
	after := loadConfig(t)
	if want := []string{"me@work.example", "old@laptop.example"}; !slices.Equal(after.Emails, want) {
		t.Errorf("emails = %q, want %q", after.Emails, want)
	}
	if after.Mirror != before.Mirror || after.Author != before.Author {
		t.Errorf("mirror %+v author %+v changed, want %+v %+v", after.Mirror, after.Author, before.Mirror, before.Author)
	}
	if _, s := homeFiles(t); !bytes.Equal(s, secret) {
		t.Error("secret file changed")
	}
}

// One bad value aborts the whole edit, so a typo never half-applies.
func TestEditFailureWritesNothing(t *testing.T) {
	gittest.Need(t)
	work := t.TempDir()
	hosted, notGit := filepath.Join(work, "hosted"), filepath.Join(work, "plain")
	gittest.Init(t, hosted)
	gittest.Git(t, hosted, nil, "remote", "add", "origin", "https://github.com/acme/hosted.git")
	if err := os.Mkdir(notGit, 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		edit   func() error
		wanted string // substring of the error
	}{
		{"email without @", func() error { return Add("email", []string{"old@laptop.example", "oldlaptop"}, io.Discard) }, "oldlaptop"},
		{"repo with a github.com remote", func() error { return Add("repo", []string{hosted}, io.Discard) }, "hosted"},
		{"folder that is not a repo", func() error { return Add("repo", []string{notGit}, io.Discard) }, "plain"},
		{"scan folder with no repos", func() error { return Add("scan", []string{notGit}, io.Discard) }, "no git repos"},
		{"unknown kind", func() error { return Add("mirror", []string{"x"}, io.Discard) }, "email, repo, or scan"},
		{"no values", func() error { return Add("email", nil, io.Discard) }, "gbridge add email"},
		{"email not in config", func() error { return Remove("email", []string{"nobody@work.example"}, io.Discard) }, "nobody@work.example"},
		{"last email", func() error { return Remove("email", []string{"ME@work.example"}, io.Discard) }, "gbridge add email"},
		{"repo not in config", func() error { return Remove("repo", []string{hosted}, io.Discard) }, "hosted"},
		{"scan folder not in config", func() error { return Remove("scan", []string{work}, io.Discard) }, "not a scan folder in config"},
		{"unknown remove kind", func() error { return Remove("mirror", []string{"x"}, io.Discard) }, "email, repo, or scan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctorHome(t, homeFull, nil)
			cfg, secret := homeFiles(t)
			if err := tt.edit(); err == nil || !strings.Contains(err.Error(), tt.wanted) {
				t.Errorf("got %v, want an error containing %q", err, tt.wanted)
			}
			if c, s := homeFiles(t); !bytes.Equal(c, cfg) || !bytes.Equal(s, secret) {
				t.Error("config or secret changed")
			}
		})
	}
}

// A repo named on the command line is kept without commits by the configured
// emails, as in init. A scan folder is kept as a folder, so repos cloned into
// it later are found; the repos it holds now are not copied into repos.
func TestAddRepoAndScan(t *testing.T) {
	gittest.Need(t)
	doctorHome(t, homeFull, nil)
	work := t.TempDir()
	team := filepath.Join(work, "team")
	empty, mine, theirs := filepath.Join(work, "empty"), filepath.Join(team, "mine"), filepath.Join(team, "theirs")
	for _, r := range []string{empty, mine, theirs} {
		gittest.Init(t, r)
	}
	gittest.Commit(t, mine, "Me@Work.example", "2025-01-02T10:00:00Z")
	gittest.Commit(t, theirs, "someone@work.example", "2025-01-02T10:00:00Z")

	if err := Add("repo", []string{empty}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Add("scan", []string{team + string(filepath.Separator)}, &out); err != nil {
		t.Fatalf("Add(scan) = %v\n%s", err, out.String())
	}
	cfg := loadConfig(t)
	if !slices.Equal(cfg.Repos, []string{empty}) || !slices.Equal(cfg.Scan, []string{team}) {
		t.Errorf("repos %q scan %q, want [%s] [%s]\n%s", cfg.Repos, cfg.Scan, empty, team, out.String())
	}
	if got := out.String(); !strings.Contains(got, "found "+mine) || strings.Contains(got, "found "+theirs) {
		t.Errorf("output does not show what the scan folder holds now:\n%s", got)
	}
	// What add showed is not news to the next plan.
	if seen, err := config.LoadSeen(); err != nil || !seen[mine] || seen[theirs] {
		t.Errorf("seen = %v, %v; want %s only", seen, err, mine)
	}

	// Adding the folder again changes nothing and says so, without failing.
	out.Reset()
	if err := Add("scan", []string{team}, &out); err != nil {
		t.Fatalf("second Add(scan) = %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, "have  "+team) || !strings.Contains(got, "nothing new") {
		t.Errorf("second scan output does not report the listed folder and no change:\n%s", got)
	}
	if cfg := loadConfig(t); !slices.Equal(cfg.Scan, []string{team}) {
		t.Errorf("scan after second add = %q, want [%s]", cfg.Scan, team)
	}
}

// Doctor tells the user to remove a clone or scan folder that no longer
// exists, so remove has to work on a path that is gone.
func TestRemoveRepoAndEmail(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "deleted-clone")
	kept := filepath.Join(t.TempDir(), "kept")
	doctorHome(t, homeFull, func(c *config.Config) {
		c.Emails = []string{"me@work.example", "old@laptop.example"}
		c.Repos = []string{gone, kept}
		c.Scan = []string{gone, kept}
	})
	_, secret := homeFiles(t)

	if err := Remove("repo", []string{gone}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := Remove("email", []string{"OLD@laptop.example"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := Remove("scan", []string{gone}, io.Discard); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t)
	if !slices.Equal(cfg.Scan, []string{kept}) {
		t.Errorf("scan %q, want [%s]", cfg.Scan, kept)
	}
	if !slices.Equal(cfg.Repos, []string{kept}) || !slices.Equal(cfg.Emails, []string{"me@work.example"}) {
		t.Errorf("repos %q emails %q, want [%s] [me@work.example]", cfg.Repos, cfg.Emails, kept)
	}
	if _, s := homeFiles(t); !bytes.Equal(s, secret) {
		t.Error("secret file changed")
	}
}

// gbridge config is what a user reads to see why a clone isn't mirrored.
func TestConfigReport(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{
		Version: 1,
		Emails:  []string{"me@work.com", "alt@work.local"},
		Repos:   []string{filepath.Join(tmp, "work", "api"), filepath.Join(tmp, "work", "web")},
		Scan:    []string{filepath.Join(tmp, "clones"), filepath.Join(tmp, "oss")},
		Mirror: config.Mirror{
			Repo: testMirror, Remote: "https://github.com/" + testMirror + ".git",
			Path: filepath.Join(tmp, "mirror.git"), Branch: "main", Visibility: "private",
		},
		Author:     config.Author{Name: testLogin, Email: testNoreply},
		TimePolicy: ident.PolicyExact,
		Identity:   ident.ModeAuthorDate,
	}
	var b bytes.Buffer
	writeConfig(&b, filepath.Join(tmp, "config.json"), cfg)
	cfg.Repos, cfg.Scan, cfg.Mirror.Repo, cfg.Mirror.Remote, cfg.IncludeGitHubRepos = nil, nil, "", "", true
	b.WriteString("---\n")
	writeConfig(&b, filepath.Join(tmp, "config.json"), cfg)
	golden(t, "config.golden", filepath.ToSlash(strings.ReplaceAll(b.String(), tmp, "$TMP")))
}
