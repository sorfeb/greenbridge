package hook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sorfeb/greenbridge/internal/gitx"
)

// Files in a repo's hooks folder. pre-commit moves an existing pre-push to
// pre-push.legacy and runs it; lefthook moves it to pre-push.old and never
// runs it.
const (
	scriptName  = "pre-push"
	chainedName = "pre-push.gbridge-orig" // the repo's own pre-push, run after ours
	legacyName  = "pre-push.legacy"
	oldName     = "pre-push.old"
	marker      = "# gbridge push hook"
)

// ErrHooksPath is returned for a repo whose core.hooksPath (husky, for
// example) points git away from .git/hooks.
var ErrHooksPath = errors.New("core.hooksPath is set, so git never runs .git/hooks here")

// State is how a push in one source repo reaches the push hook.
type State int

// The states a source repo's push hook coverage can be in.
const (
	Uncovered     State = iota
	CoveredGlobal       // the hook in global git config runs
	CoveredRepo         // our pre-push script runs
	Disabled            // hook.gbridge.enabled is false in this repo
	Overridden          // lefthook moved our script to pre-push.old
	HooksPath           // core.hooksPath is set and no global hook runs
)

func (s State) String() string {
	return [...]string{"not covered", "covered", "covered", "opted out", "overridden", "not covered"}[s]
}

// Covered reports whether a push in the repo starts a background sync.
func (s State) Covered() bool { return s == CoveredGlobal || s == CoveredRepo }

// Coverage is one source repo's push hook state.
type Coverage struct {
	Repo  string
	State State
	Exe   string // binary our pre-push script runs; empty for the global hook
}

// Script is the pre-push script for one repo. It runs the same guarded
// Command as the global hook, unless the repo opted out, then hands the
// push to the repo's own pre-push, untouched: ours never reads stdin.
func Script(exe string) string {
	return "#!/usr/bin/env sh\n" +
		marker + ": starts a background gbridge sync and never fails the push.\n" +
		"# Written by gbridge hook install --per-repo; gbridge hook uninstall removes it.\n" +
		`if [ "$(git config --type=bool hook.` + Name + `.enabled 2>/dev/null)" != false ]; then` + "\n" +
		"\t" + Command(exe) + "\n" +
		"fi\n" +
		`orig="$(dirname "$0")/` + chainedName + `"` + "\n" +
		`if [ -x "$orig" ]; then exec "$orig" "$@"; fi` + "\n"
}

// layout reads where repo keeps hooks and the config that decides whether
// they run, last value winning as git does.
func layout(repo string) (hooks, hooksPath string, enabled bool, err error) {
	common, err := gitx.Git(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", false, err
	}
	out, err := configGet(repo, "--get-regexp", `^(core\.hookspath|hook\.`+Name+`\.enabled)$`)
	if err != nil {
		return "", "", false, err
	}
	enabled = true
	for line := range strings.SplitSeq(out, "\n") {
		key, val, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "core.hookspath":
			hooksPath = val
		case "hook." + Name + ".enabled":
			enabled = !isFalse(val)
		}
	}
	return filepath.Join(filepath.FromSlash(common), "hooks"), hooksPath, enabled, nil
}

// ours reports whether the file at p is our script, and the binary it runs.
func ours(p string) (exe string, ok bool) {
	b, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(b), marker) {
		return "", false
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		if e, ok := CommandPath(strings.TrimSpace(line)); ok {
			return e, true
		}
	}
	return "", true
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// Inspect reports whether a push in repo starts a background sync. global
// says whether the global hook runs at all: installed, on Git 2.54 or later.
func Inspect(repo string, global bool) (Coverage, error) {
	c := Coverage{Repo: repo}
	hooks, hooksPath, enabled, err := layout(repo)
	if err != nil {
		return c, err
	}
	switch {
	case global:
		c.State = CoveredGlobal
	case hooksPath != "":
		c.State = HooksPath
	default:
		for _, n := range []string{scriptName, legacyName} {
			if exe, ok := ours(filepath.Join(hooks, n)); ok {
				c.State, c.Exe = CoveredRepo, exe
				break
			}
		}
		if _, ok := ours(filepath.Join(hooks, oldName)); ok && c.State == Uncovered {
			c.State = Overridden
		}
	}
	if c.State.Covered() && !enabled {
		c.State = Disabled
	}
	return c, nil
}

// InstallRepo writes our pre-push script into repo's hooks folder. An
// existing pre-push that isn't ours moves to pre-push.gbridge-orig, and our
// script runs it after itself. Installing again only rewrites our script.
func InstallRepo(repo, exe string) error {
	hooks, hooksPath, _, err := layout(repo)
	if err != nil {
		return err
	}
	if hooksPath != "" {
		return fmt.Errorf("%w (core.hooksPath = %s)", ErrHooksPath, hooksPath)
	}
	if err := os.MkdirAll(hooks, 0o750); err != nil {
		return err
	}
	// pre-commit already runs our script from pre-push.legacy.
	if legacy := filepath.Join(hooks, legacyName); exists(legacy) {
		if _, ok := ours(legacy); ok {
			return writeScript(legacy, exe)
		}
	}
	script, chained := filepath.Join(hooks, scriptName), filepath.Join(hooks, chainedName)
	if _, ok := ours(script); !ok && exists(script) {
		if exists(chained) {
			return fmt.Errorf("%s and %s both exist in %s; move one aside first", scriptName, chainedName, hooks)
		}
		if err := os.Rename(script, chained); err != nil {
			return err
		}
	}
	return writeScript(script, exe)
}

func writeScript(p, exe string) error {
	// Git runs only executable hooks; the script holds nothing secret.
	if err := os.WriteFile(p, []byte(Script(exe)), 0o755); err != nil { //nolint:gosec // G306: git runs only executable hooks
		return err
	}
	return os.Chmod(p, 0o755) //nolint:gosec // G302: WriteFile keeps an existing file's mode; git runs only executable hooks
}

// UninstallRepo removes our script from repo, wherever a hook manager moved
// it, and puts the repo's own pre-push back where ours was. removed is false
// when the repo had none of ours.
func UninstallRepo(repo string) (removed bool, err error) {
	hooks, _, _, err := layout(repo)
	if err != nil {
		return false, err
	}
	chained := filepath.Join(hooks, chainedName)
	for _, n := range []string{scriptName, legacyName, oldName} {
		p := filepath.Join(hooks, n)
		if _, ok := ours(p); !ok {
			continue
		}
		if err := os.Remove(p); err != nil {
			return removed, err
		}
		removed = true
		if exists(chained) {
			if err := os.Rename(chained, p); err != nil {
				return removed, err
			}
		}
	}
	return removed, nil
}
