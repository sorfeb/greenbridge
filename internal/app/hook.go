package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/gitx"
	"github.com/sorfeb/greenbridge/internal/hook"
)

// PrePush is what the push hook runs, in the repo at dir: start runs when
// dir is a source repo, or a linked worktree of one. The global push hook
// fires in every repo on the machine, and in the mirror clone when a
// background sync pushes, so everything else returns without starting.
func PrePush(dir string, start func() error) (started bool, err error) {
	if os.Getenv(hook.BackgroundEnv) == "1" {
		return false, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return false, err
	}
	if !pushedSourceRepo(cfg, dir) {
		return false, nil
	}
	return true, start()
}

// pushedSourceRepo reports whether dir is a source repo. Path checks come
// first, since they cost no git call; a linked worktree needs one to find
// its main repo. Git sets GIT_DIR for a hook in a worktree, and the call
// inherits it.
func pushedSourceRepo(cfg *config.Config, dir string) bool {
	if isSource(cfg, dir) {
		return true
	}
	common, err := gitx.Git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false
	}
	root := filepath.Dir(filepath.FromSlash(common))
	return !samePath(root, dir) && isSource(cfg, root)
}

func isSource(cfg *config.Config, dir string) bool {
	if samePath(dir, cfg.Mirror.Path) {
		return false
	}
	for _, r := range cfg.Repos {
		if samePath(dir, r) {
			return true
		}
	}
	for _, s := range cfg.ScanFolders {
		for d := dir; ; d = filepath.Dir(d) {
			if samePath(d, s) {
				return true
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	return false
}

// samePath reports whether a and b name the same existing folder, whatever
// their case, separators, or links.
func samePath(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

// HookInstall installs the push hook that runs exe: in global git config,
// or with perRepo in each source repo's .git/hooks.
func HookInstall(exe string, perRepo bool, out io.Writer) error {
	v, err := hook.GitVersion()
	if err != nil {
		return err
	}
	return hookInstall(v, exe, perRepo, out)
}

func hookInstall(v hook.Version, exe string, perRepo bool, out io.Writer) error {
	home, err := config.Dir()
	if err != nil {
		return err
	}
	logPath := hook.FilesIn(home).Log
	if perRepo {
		return installPerRepo(v, exe, logPath, out)
	}
	if !v.ConfigHooks() {
		return fmt.Errorf("git %s ignores hooks in git config, so a push hook there would never run (needs Git 2.54 or later).\n"+
			"Upgrade Git, or run gbridge hook install --per-repo to write a pre-push hook into each source repo's .git/hooks", v)
	}
	if err := hook.InstallGlobal(exe); err != nil {
		return err
	}
	fmt.Fprintln(out, "push hook installed in global git config: every git push from this machine starts a background sync.")
	fmt.Fprintln(out, "No repo's files changed, so nobody else sees it.")
	fmt.Fprintf(out, "  runs     %s\n", exe)
	fmt.Fprintf(out, "  log      %s\n", logPath)
	fmt.Fprintln(out, "  opt out  git config hook.gbridge.enabled false   (in the repo to leave out)")
	if _, err := config.Load(); errors.Is(err, config.ErrNoConfig) {
		fmt.Fprintln(out, "No config yet, so pushes start nothing until you run gbridge init.")
	}
	return nil
}

func installPerRepo(v hook.Version, exe, logPath string, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if v.ConfigHooks() {
		fmt.Fprintf(out, "git %s runs hooks from git config; gbridge hook install without --per-repo needs no per-repo files.\n", v)
	}
	repos, _ := sources(cfg)
	installed, failed := 0, 0
	for _, r := range repos {
		switch err := hook.InstallRepo(r.path, exe); {
		case errors.Is(err, hook.ErrHooksPath):
			fmt.Fprintf(out, "  skipped    %s: %v\n", r.path, err)
		case err != nil:
			failed++
			fmt.Fprintf(out, "  failed     %s: %v\n", r.path, err)
		default:
			installed++
			fmt.Fprintf(out, "  installed  %s\n", r.path)
		}
	}
	fmt.Fprintf(out, "push hook installed in %d of %d source repos (.git/hooks/pre-push, never pushed or cloned).\n", installed, len(repos))
	fmt.Fprintf(out, "  runs     %s\n", exe)
	fmt.Fprintf(out, "  log      %s\n", logPath)
	fmt.Fprintln(out, "Source repos added later need gbridge hook install --per-repo again.")
	if failed > 0 {
		return fmt.Errorf("push hook not installed in %d source repos", failed)
	}
	return nil
}

// HookUninstall removes the push hook from global git config and from every
// source repo.
func HookUninstall(out io.Writer) error {
	removed, err := hook.UninstallGlobal()
	if err != nil {
		return err
	}
	if removed {
		fmt.Fprintln(out, "push hook removed from global git config")
	}
	if cfg, err := config.Load(); err == nil {
		repos, _ := sources(cfg)
		for _, r := range repos {
			ok, err := hook.UninstallRepo(r.path)
			switch {
			case err != nil:
				fmt.Fprintf(out, "could not remove the push hook from %s: %v\n", r.path, err)
			case ok:
				removed = true
				fmt.Fprintf(out, "push hook removed from %s\n", r.path)
			}
		}
	}
	if !removed {
		fmt.Fprintln(out, "no push hook to remove")
	}
	return nil
}

// hookReport is everything known about the push hook on this machine.
type hookReport struct {
	version    hook.Version
	versionErr error
	global     hook.Global
	globalErr  error
	repos      []hook.Coverage // nil without a config
	log        string
	last       hook.Result
	hasLast    bool
	lastErr    error
}

// globalActive reports whether the global push hook runs on pushes.
func (h hookReport) globalActive() bool { return h.global.Installed() && h.version.ConfigHooks() }

// gatherHooks reads the push hook from git config, each source repo of cfg
// (nil for none), and the background sync log.
func gatherHooks(cfg *config.Config) hookReport {
	var h hookReport
	h.version, h.versionErr = hook.GitVersion()
	h.global, h.globalErr = hook.ReadGlobal()
	if home, err := config.Dir(); err == nil {
		h.log = hook.FilesIn(home).Log
		h.last, h.hasLast, h.lastErr = hook.LastResult(h.log)
	}
	if cfg == nil {
		return h
	}
	repos, _ := sources(cfg)
	for _, r := range repos {
		c, err := hook.Inspect(r.path, h.globalActive())
		if err != nil {
			continue // a missing or broken repo is the repos check's to report
		}
		h.repos = append(h.repos, c)
	}
	return h
}

// stateDetail says why a push in a repo in state s starts no background
// sync, and the fix.
func stateDetail(s hook.State) (why, fix string) {
	switch s {
	case hook.HooksPath:
		return "core.hooksPath is set (husky or similar), so .git/hooks never runs",
			"upgrade Git to 2.54 or later and run gbridge hook install"
	case hook.Overridden:
		return "a hook manager (lefthook) moved the push hook to pre-push.old and never runs it",
			"gbridge hook install --per-repo, after the hook manager's own install"
	case hook.Disabled:
		return "opted out with hook.gbridge.enabled false", ""
	}
	return "no push hook", "gbridge hook install --per-repo, or Git 2.54 or later and gbridge hook install"
}

// HookStatus prints the push hook, which source repos it covers, and the
// last background sync.
func HookStatus(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil && !errors.Is(err, config.ErrNoConfig) {
		return err
	}
	h := gatherHooks(cfg)
	if h.versionErr != nil {
		return h.versionErr
	}
	if h.globalErr != nil {
		return h.globalErr
	}
	if h.version.ConfigHooks() {
		fmt.Fprintf(out, "git %s runs hooks from global git config\n", h.version)
	} else {
		fmt.Fprintf(out, "git %s ignores hooks in git config (needs 2.54 or later); only per-repo hooks run\n", h.version)
	}
	switch {
	case h.global.Installed():
		exe, _ := h.global.Exe()
		fmt.Fprintf(out, "push hook in global git config runs %s%s\n", exe, missing(exe))
	case h.global.Set():
		fmt.Fprintln(out, "push hook in global git config is half written: run gbridge hook install or gbridge hook uninstall")
	default:
		fmt.Fprintln(out, "no push hook in global git config")
	}
	if cfg == nil {
		fmt.Fprintln(out, "no config, so no source repos: run gbridge init")
	}
	if len(h.repos) > 0 {
		fmt.Fprintln(out, "source repos:")
	}
	for _, c := range h.repos {
		line := fmt.Sprintf("  %-12s %s", c.State, c.Repo)
		switch {
		case c.State == hook.CoveredRepo:
			line += " (.git/hooks/pre-push runs " + c.Exe + missing(c.Exe) + ")"
		case c.State == hook.Disabled:
			line += " (hook.gbridge.enabled false)"
		case !c.State.Covered():
			why, _ := stateDetail(c.State)
			line += ": " + why
		}
		fmt.Fprintln(out, line)
	}
	if h.hasLast {
		fmt.Fprintf(out, "last background sync %s\n", lastLine(h.last))
	} else {
		fmt.Fprintln(out, "no background sync yet")
	}
	fmt.Fprintf(out, "log: %s\n", h.log)
	return nil
}

func missing(exe string) string {
	if hook.Exists(exe) {
		return ""
	}
	return ", which is gone"
}

func lastLine(r hook.Result) string {
	outcome := "ok"
	if !r.OK {
		outcome = "failed"
	}
	return fmt.Sprintf("%s %s: %s", r.Time.Local().Format(time.DateTime), outcome, r.Msg)
}
