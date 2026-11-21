package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/hook"
)

func TestCheckHook(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone", "gbridge")
	here := filepath.Join(t.TempDir(), "gbridge")
	if err := os.WriteFile(here, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	global := func(exe string) hook.Global {
		return hook.Global{Command: hook.Command(exe), Events: []string{hook.Event}}
	}
	repo := func(s hook.State, exe string) hook.Coverage { return hook.Coverage{Repo: "/w/api", State: s, Exe: exe} }
	for _, tc := range []struct {
		name string
		h    hookReport
		want []status // in order
	}{
		{
			name: "nothing installed",
			h:    hookReport{version: newGit, repos: []hook.Coverage{repo(hook.Uncovered, "")}},
			want: []status{statusWarn},
		},
		{
			name: "global on new Git",
			h:    hookReport{version: newGit, global: global(here), repos: []hook.Coverage{repo(hook.CoveredGlobal, "")}},
			want: []status{statusOK, statusOK},
		},
		{
			name: "global on old Git",
			h:    hookReport{version: oldGit, global: global(here), repos: []hook.Coverage{repo(hook.Uncovered, "")}},
			want: []status{statusFail, statusWarn},
		},
		{
			name: "global binary gone",
			h:    hookReport{version: newGit, global: global(gone), repos: []hook.Coverage{repo(hook.CoveredGlobal, "")}},
			want: []status{statusFail, statusOK},
		},
		{
			name: "half-written global",
			h:    hookReport{version: newGit, global: hook.Global{Events: []string{hook.Event}}},
			want: []status{statusFail},
		},
		{
			name: "per repo, binary gone",
			h:    hookReport{version: oldGit, repos: []hook.Coverage{repo(hook.CoveredRepo, gone)}},
			want: []status{statusFail, statusOK},
		},
		{
			name: "opted out",
			h:    hookReport{version: newGit, global: global(here), repos: []hook.Coverage{repo(hook.Disabled, "")}},
			want: []status{statusOK, statusOK},
		},
		{
			name: "lefthook took over",
			h:    hookReport{version: oldGit, repos: []hook.Coverage{repo(hook.Overridden, here)}},
			want: []status{statusOK, statusWarn},
		},
		{
			name: "last background sync failed",
			h: hookReport{
				version: newGit, global: global(here), repos: []hook.Coverage{repo(hook.CoveredGlobal, "")},
				hasLast: true, last: hook.Result{Msg: "push failed"}, log: "/home/sync.log",
			},
			want: []status{statusOK, statusOK, statusWarn},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := checkHook(tc.h)
			var got []status
			for _, r := range rs {
				got = append(got, r.status)
				if r.check != "hook" {
					t.Errorf("check name %q, want hook", r.check)
				}
				if (r.status == statusFail || r.status == statusWarn) && r.fix == "" {
					t.Errorf("%s %q has no fix", r.status, r.msg)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("statuses %v, want %v: %+v", got, tc.want, rs)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("statuses %v, want %v: %+v", got, tc.want, rs)
					break
				}
			}
		})
	}
}

// Doctor reads the push hook from git and the log, and a hook pointing at a
// binary that is gone fails the check.
func TestDoctorHookGather(t *testing.T) {
	gittest.Need(t)
	isolateGitConfig(t, "")
	tmp := t.TempDir()
	cfg := mirrored(t, tmp)
	if err := hook.InstallGlobal(filepath.Join(tmp, "gone", "gbridge")); err != nil {
		t.Fatal(err)
	}
	h := gatherHooks(cfg)
	if !h.global.Installed() || len(h.repos) != 1 {
		t.Fatalf("gathered %+v, want the global hook and one source repo", h)
	}
	if !h.version.ConfigHooks() && h.repos[0].State != hook.Uncovered {
		t.Errorf("old git: %+v, want uncovered", h.repos[0])
	}
	if h.version.ConfigHooks() && h.repos[0].State != hook.CoveredGlobal {
		t.Errorf("new git: %+v, want covered by the global hook", h.repos[0])
	}
	if rs := checkHook(h); rs[0].status != statusFail {
		t.Errorf("first result %+v, want a failure (binary gone or Git too old)", rs[0])
	}
}
