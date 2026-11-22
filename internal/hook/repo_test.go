package hook

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/gitx"
)

// pushable makes a source repo with one commit and a bare origin it can push
// to. It returns the repo and its hooks folder.
func pushable(t *testing.T) (repo, hooks string) {
	t.Helper()
	dir := t.TempDir()
	repo, remote := filepath.Join(dir, "src"), filepath.Join(dir, "origin.git")
	gittest.InitBare(t, remote)
	gittest.Init(t, repo)
	gittest.Git(t, repo, nil, "remote", "add", "origin", remote)
	gittest.Commit(t, repo, "me@work.example", "2025-01-02T10:00:00Z")
	return repo, filepath.Join(repo, ".git", "hooks")
}

func push(t *testing.T, repo string) error {
	t.Helper()
	gittest.Commit(t, repo, "me@work.example", "2025-01-03T10:00:00Z")
	_, err := gitx.Git(repo, "push", "-q", "origin", "HEAD:main")
	return err
}

// writeHook writes an executable pre-push hook that appends a line to a file
// next to it, then exits with code.
func writeHook(t *testing.T, hooks, name, code string) string {
	t.Helper()
	ran := filepath.Join(hooks, name+".ran")
	body := "#!/bin/sh\necho ran >> '" + filepath.ToSlash(ran) + "'\nexit " + code + "\n"
	if err := os.MkdirAll(hooks, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, name), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return ran
}

func ran(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestInstallRepoFresh(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	repo, hooks := pushable(t)
	exe := filepath.Join(t.TempDir(), "with space", "gbridge")
	if err := InstallRepo(repo, exe); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(hooks, "pre-push"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "#!/usr/bin/env sh\n") {
		t.Errorf("script starts %q, want the env shebang (pre-commit on Windows fails on /bin/sh)", strings.SplitN(string(b), "\n", 2)[0])
	}
	c, err := Inspect(repo, false)
	if err != nil || c.State != CoveredRepo || c.Exe != exe {
		t.Errorf("Inspect = %+v, %v; want CoveredRepo running %q", c, err, exe)
	}
	// exe does not exist: the push must still succeed.
	if err := push(t, repo); err != nil {
		t.Errorf("push with the binary missing failed: %v", err)
	}
}

// A repo's own pre-push (git-lfs ships one) keeps running after ours, with
// its exit code: a failing repo hook still fails the push, as it should.
func TestInstallRepoChains(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	repo, hooks := pushable(t)
	theirs := writeHook(t, hooks, "pre-push", "0")
	exe := filepath.Join(t.TempDir(), "gbridge")

	if err := InstallRepo(repo, exe); err != nil {
		t.Fatal(err)
	}
	if err := InstallRepo(repo, exe); err != nil { // idempotent: never chains itself
		t.Fatal(err)
	}
	if err := push(t, repo); err != nil {
		t.Fatalf("push: %v", err)
	}
	if !ran(theirs) {
		t.Error("the repo's own pre-push did not run")
	}
	if b, _ := os.ReadFile(filepath.Join(hooks, chainedName)); strings.Contains(string(b), marker) {
		t.Error("second install chained our own script")
	}

	writeHook(t, hooks, chainedName, "1")
	if err := push(t, repo); err == nil {
		t.Error("push succeeded though the repo's own pre-push exited 1")
	}

	removed, err := UninstallRepo(repo)
	if err != nil || !removed {
		t.Fatalf("UninstallRepo = %v, %v", removed, err)
	}
	b, err := os.ReadFile(filepath.Join(hooks, "pre-push"))
	if err != nil || strings.Contains(string(b), marker) {
		t.Errorf("after uninstall pre-push = %q, %v; want the repo's own hook back", b, err)
	}
	if _, err := os.Stat(filepath.Join(hooks, chainedName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s left behind: %v", chainedName, err)
	}
}

func TestInstallRepoHooksPath(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	repo, _ := pushable(t)
	gittest.Git(t, repo, nil, "config", "core.hooksPath", ".husky/_")
	if err := InstallRepo(repo, "/x/gbridge"); !errors.Is(err, ErrHooksPath) {
		t.Errorf("InstallRepo with core.hooksPath = %v, want ErrHooksPath", err)
	}
	c, err := Inspect(repo, false)
	if err != nil || c.State != HooksPath {
		t.Errorf("Inspect = %+v, %v; want HooksPath", c, err)
	}
	if c, _ := Inspect(repo, true); c.State != CoveredGlobal {
		t.Errorf("with the global hook: %+v, want CoveredGlobal (config hooks run beside core.hooksPath)", c)
	}
}

func TestInspect(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	for _, tc := range []struct {
		name   string
		global bool
		setup  func(t *testing.T, repo, hooks string)
		want   State
	}{
		{name: "nothing", want: Uncovered},
		{name: "global", global: true, want: CoveredGlobal},
		{
			name: "global, opted out", global: true, want: Disabled,
			setup: func(t *testing.T, repo, _ string) {
				t.Helper()
				gittest.Git(t, repo, nil, "config", "hook.gbridge.enabled", "false")
			},
		},
		{
			name: "per repo, opted out", want: Disabled,
			setup: func(t *testing.T, repo, _ string) {
				t.Helper()
				mustInstall(t, repo)
				gittest.Git(t, repo, nil, "config", "hook.gbridge.enabled", "off")
			},
		},
		{
			name: "global, opted in explicitly", global: true, want: CoveredGlobal,
			setup: func(t *testing.T, repo, _ string) {
				t.Helper()
				gittest.Git(t, repo, nil, "config", "hook.gbridge.enabled", "yes")
			},
		},
		{
			name: "moved aside by pre-commit, which still runs it", want: CoveredRepo,
			setup: func(t *testing.T, repo, hooks string) {
				t.Helper()
				mustInstall(t, repo)
				rename(t, hooks, "pre-push", "pre-push.legacy")
				writeHook(t, hooks, "pre-push", "0")
			},
		},
		{
			name: "moved aside by lefthook, which never runs it", want: Overridden,
			setup: func(t *testing.T, repo, hooks string) {
				t.Helper()
				mustInstall(t, repo)
				rename(t, hooks, "pre-push", "pre-push.old")
				writeHook(t, hooks, "pre-push", "0")
			},
		},
		{
			name: "someone else's pre-push", want: Uncovered,
			setup: func(t *testing.T, _, hooks string) {
				t.Helper()
				writeHook(t, hooks, "pre-push", "0")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, hooks := pushable(t)
			if tc.setup != nil {
				tc.setup(t, repo, hooks)
			}
			c, err := Inspect(repo, tc.global)
			if err != nil || c.State != tc.want {
				t.Errorf("Inspect = %+v, %v; want %v", c, err, tc.want)
			}
		})
	}
}

func mustInstall(t *testing.T, repo string) {
	t.Helper()
	if err := InstallRepo(repo, "/x/gbridge"); err != nil {
		t.Fatal(err)
	}
}

func rename(t *testing.T, dir, from, to string) {
	t.Helper()
	if err := os.Rename(filepath.Join(dir, from), filepath.Join(dir, to)); err != nil {
		t.Fatal(err)
	}
}

// Uninstall finds our script where a hook manager moved it.
func TestUninstallRepoMovedAside(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	repo, hooks := pushable(t)
	mustInstall(t, repo)
	rename(t, hooks, "pre-push", "pre-push.legacy")
	theirs := writeHook(t, hooks, "pre-push", "0")
	removed, err := UninstallRepo(repo)
	if err != nil || !removed {
		t.Fatalf("UninstallRepo = %v, %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(hooks, "pre-push.legacy")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("pre-push.legacy left behind: %v", err)
	}
	if err := push(t, repo); err != nil || !ran(theirs) {
		t.Errorf("push after uninstall: %v, manager's hook ran %v", err, ran(theirs))
	}
}

func TestUninstallRepoNothing(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	repo, hooks := pushable(t)
	writeHook(t, hooks, "pre-push", "0")
	removed, err := UninstallRepo(repo)
	if err != nil || removed {
		t.Errorf("UninstallRepo on someone else's hook = %v, %v; want untouched", removed, err)
	}
	if _, err := os.Stat(filepath.Join(hooks, "pre-push")); err != nil {
		t.Errorf("someone else's hook removed: %v", err)
	}
}
