package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/hook"
)

var (
	oldGit = hook.Version{Major: 2, Minor: 51}
	newGit = hook.Version{Major: 2, Minor: 54}
)

// The global push hook fires in every repo on the machine. Only a push in a
// source repo, or in a linked worktree of one, starts a background sync.
func TestPrePushStartsOnlyForSourceRepos(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "")
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	listed := cfg.Repos[0]
	gittest.Commit(t, listed, "me@work.com", "2025-01-02T10:00:00Z")
	scan := filepath.Join(tmp, "scan")
	found := filepath.Join(scan, "team", "found")
	gittest.Init(t, found)
	cfg.ScanFolders = []string{scan}
	other := filepath.Join(tmp, "other")
	gittest.Init(t, other)
	worktree := filepath.Join(tmp, "wt")
	gittest.Git(t, listed, nil, "worktree", "add", "-q", "-b", "side", worktree)
	gittest.InitBare(t, cfg.Mirror.Path)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, dir string
		want      bool
	}{
		{"listed repo", listed, true},
		{"repo in a scan folder", found, true},
		{"worktree of a listed repo", worktree, true},
		{"unrelated repo", other, false},
		{"mirror clone", cfg.Mirror.Path, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := false
			got, err := PrePush(tc.dir, func() error { started = true; return nil })
			if err != nil || got != tc.want || started != tc.want {
				t.Errorf("PrePush(%s) = %v, %v (started %v); want %v", tc.dir, got, err, started, tc.want)
			}
		})
	}

	// The background sync's own push to the mirror repo fires the hook too.
	t.Setenv(hook.BackgroundEnv, "1")
	if got, _ := PrePush(listed, func() error { t.Error("started from a background sync"); return nil }); got {
		t.Error("PrePush inside a background sync started another")
	}
}

// Git before 2.54 ignores hook.* in config without a word, so install must
// refuse rather than report a hook that never runs.
func TestHookInstallRefusesOldGit(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "")
	gcfg := os.Getenv("GIT_CONFIG_GLOBAL")
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	var out bytes.Buffer
	err := hookInstall(oldGit, "/opt/gbridge", false, &out)
	if err == nil {
		t.Fatalf("install on Git 2.51 succeeded:\n%s", out.String())
	}
	for _, want := range []string{"2.51.0", "2.54", "--per-repo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if b, _ := os.ReadFile(gcfg); strings.Contains(string(b), "gbridge") {
		t.Errorf("global config written anyway:\n%s", b)
	}
}

func TestHookInstallGlobal(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "")
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	exe := filepath.Join(t.TempDir(), "gbridge")
	var out bytes.Buffer
	if err := hookInstall(newGit, exe, false, &out); err != nil {
		t.Fatal(err)
	}
	g, err := hook.ReadGlobal()
	if p, ok := g.Exe(); err != nil || !g.Installed() || !ok || p != exe {
		t.Fatalf("global hook = %+v, %v; want installed running %s", g, err, exe)
	}
	for _, want := range []string{"global git config", "hook.gbridge.enabled false", "sync.log"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("install output lacks %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := HookUninstall(&out); err != nil {
		t.Fatal(err)
	}
	if g, _ := hook.ReadGlobal(); g.Set() {
		t.Errorf("after uninstall: %+v", g)
	}
}

func TestHookInstallPerRepo(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "")
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	husky := filepath.Join(tmp, "husky")
	gittest.Init(t, husky)
	gittest.Git(t, husky, nil, "config", "core.hooksPath", ".husky/_")
	cfg.Repos = append(cfg.Repos, husky)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(tmp, "bin", "gbridge")

	var out bytes.Buffer
	if err := hookInstall(oldGit, exe, true, &out); err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if c, err := hook.Inspect(cfg.Repos[0], false); err != nil || c.State != hook.CoveredRepo {
		t.Errorf("source repo: %+v, %v; want covered", c, err)
	}
	if !strings.Contains(out.String(), "core.hooksPath") || !strings.Contains(out.String(), "1 of 2") {
		t.Errorf("install output should name the husky repo's core.hooksPath and count 1 of 2:\n%s", out.String())
	}

	out.Reset()
	if err := HookStatus(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"covered", cfg.Repos[0], "not covered", husky} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status lacks %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := HookUninstall(&out); err != nil {
		t.Fatal(err)
	}
	if c, _ := hook.Inspect(cfg.Repos[0], false); c.State != hook.Uncovered {
		t.Errorf("after uninstall: %+v", c)
	}
}
