package app

import (
	"fmt"

	"github.com/sorfeb/greenbridge/internal/hook"
)

// checkHook reports whether pushes start a background sync: the push hook
// is installed, the Git on PATH runs it, the binary it names is still there,
// it covers every source repo, and the last background sync worked.
func checkHook(h hookReport) []result {
	const check = "hook"
	switch {
	case h.versionErr != nil:
		return []result{skipped(check, "git")}
	case h.globalErr != nil:
		return []result{fail(check, "could not read global git config: "+h.globalErr.Error(), "fix the global git config, then gbridge hook install")}
	}
	var rs []result
	perRepo, perRepoExe := 0, ""
	for _, c := range h.repos {
		if c.Exe != "" {
			perRepo++
			perRepoExe = c.Exe
		}
	}
	switch exe, _ := h.global.Exe(); {
	case h.global.Set() && !h.global.Installed():
		return []result{fail(check, "push hook in global git config is half written; an event without a command breaks every git command",
			"gbridge hook install, or gbridge hook uninstall")}
	case h.global.Installed() && !h.version.ConfigHooks():
		rs = append(rs, fail(check, fmt.Sprintf("git %s ignores the push hook in global git config (needs 2.54 or later)", h.version),
			"upgrade Git, or run gbridge hook install --per-repo"))
	case h.global.Installed() && !hook.Exists(exe):
		rs = append(rs, fail(check, "push hook runs "+exe+", which is gone, so pushes start no sync",
			"run gbridge hook install from the gbridge you use now"))
	case h.global.Installed():
		rs = append(rs, ok(check, "push hook in global git config runs "+exe))
	case perRepo > 0 && !hook.Exists(perRepoExe):
		rs = append(rs, fail(check, fmt.Sprintf("push hook in %s runs %s, which is gone, so pushes start no sync", plural(perRepo, "source repo"), perRepoExe),
			"run gbridge hook install --per-repo from the gbridge you use now"))
	case perRepo > 0:
		rs = append(rs, ok(check, fmt.Sprintf("push hook in .git/hooks of %s runs %s", plural(perRepo, "source repo"), perRepoExe)))
	default:
		fix := "gbridge hook install"
		if !h.version.ConfigHooks() {
			fix = "upgrade Git to 2.54 or later and run gbridge hook install, or gbridge hook install --per-repo"
		}
		return []result{{check, statusWarn, "no push hook, so mirroring waits until you run gbridge sync", fix}}
	}
	rs = append(rs, coverage(h.repos)...)
	if h.hasLast {
		if h.last.OK {
			rs = append(rs, ok(check, "last background sync "+lastLine(h.last)))
		} else {
			rs = append(rs, result{check, statusWarn, "last background sync " + lastLine(h.last), "details in " + h.log})
		}
	}
	return rs
}

// coverage warns about each source repo a push in starts no background sync,
// or says that all are covered.
func coverage(repos []hook.Coverage) []result {
	const check = "hook"
	if len(repos) == 0 {
		return nil
	}
	var rs []result
	off := 0
	for _, c := range repos {
		switch {
		case c.State == hook.Disabled:
			off++
		case !c.State.Covered():
			why, fix := stateDetail(c.State)
			rs = append(rs, result{check, statusWarn, "push in " + c.Repo + " starts no background sync: " + why, fix})
		}
	}
	if len(rs) > 0 {
		return rs
	}
	msg := "push hook covers all " + plural(len(repos), "source repo")
	if len(repos) == 1 {
		msg = "push hook covers the source repo"
	}
	if off > 0 {
		msg = fmt.Sprintf("push hook covers %d of %s, %d opted out", len(repos)-off, plural(len(repos), "source repo"), off)
	}
	return []result{ok(check, msg)}
}
