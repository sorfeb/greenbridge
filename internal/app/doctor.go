package app

import (
	"fmt"
	"io"

	"github.com/sorfeb/greenbridge/internal/ghx"
)

type status int

const (
	statusOK status = iota
	statusWarn
	statusUnverified // nothing can confirm it; the user checks by hand
	statusFail
	statusSkipped // a prerequisite failed, or --online was not given, so the check did not run
)

func (s status) String() string {
	return [...]string{"[ok]", "[warn]", "[unverified]", "[FAIL]", "[skipped]"}[s]
}

// result is one check's verdict. fix is the remedy for a failure, or where
// to look for an unverified one.
type result struct {
	check  string
	status status
	msg    string
	fix    string
}

func ok(check, msg string) result   { return result{check, statusOK, msg, ""} }
func warn(check, msg string) result { return result{check, statusWarn, msg, ""} }
func fail(check, msg, fix string) result {
	return result{check, statusFail, msg, fix}
}

func unverified(check, msg, where string) result {
	return result{check, statusUnverified, msg, where}
}

func skipped(check, needs string) result {
	return result{check, statusSkipped, check + " not checked, needs " + needs, ""}
}

// DoctorOptions are the flags of gbridge doctor.
type DoctorOptions struct {
	// Online runs the online checks, which wait on the network: today the
	// tracked remotes. Without it doctor reads only this machine and GitHub's
	// API.
	Online bool
}

// diagnose runs every check. Each reports, even when it cannot run.
func diagnose(gh ghx.GitHub, opt DoctorOptions) []result {
	f := gather(gh, opt)
	rs := []result{checkGit(f), checkConfig(f), checkSecret(f)}
	rs = append(rs, checkRepos(f)...)
	rs = append(rs, checkEmails(f)...)
	rs = append(rs, checkRemotes(f)...)
	rs = append(rs, checkHook(gatherHooks(f.cfg))...)
	return append(rs,
		checkGH(f),
		checkAuthorEmail(f),
		checkMirror(f),
		checkMirrorBranch(f),
		checkPrivateContrib(f),
	)
}

// Doctor checks every precondition for squares to appear. It returns the
// number of failures.
func Doctor(w io.Writer, gh ghx.GitHub, opt DoctorOptions) int {
	fails := 0
	for _, r := range diagnose(gh, opt) {
		fmt.Fprintf(w, "%-12s %s\n", r.status, r.msg)
		switch r.status {
		case statusFail:
			fails++
			fmt.Fprintf(w, "             fix: %s\n", r.fix)
		case statusWarn:
			if r.fix != "" {
				fmt.Fprintf(w, "             fix: %s\n", r.fix)
			}
		case statusUnverified:
			fmt.Fprintf(w, "             check: %s\n", r.fix)
		}
	}
	fmt.Fprintln(w, "\nnote: the graph can take up to 24 hours to update after a push.")
	return fails
}
