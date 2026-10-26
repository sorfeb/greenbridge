package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/gitx"
	"github.com/sorfeb/greenbridge/internal/ident"
	"github.com/sorfeb/greenbridge/internal/source"
)

// PrivateContribSettings is where the private contributions toggle lives.
const PrivateContribSettings = "https://github.com/settings/profile (Contributions & activity: Include private contributions on my profile)"

type status int

const (
	statusOK status = iota
	statusWarn
	statusUnverified // nothing can confirm it; the user checks by hand
	statusFail
	statusSkipped // a prerequisite failed, so the check could not run
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

// facts is everything doctor learns from the machine and GitHub, gathered
// once before any check runs.
type facts struct {
	git    string
	gitErr error

	cfg     *config.Config
	cfgErr  error
	cfgPath string

	secretErr  error
	secretPath string

	repos []repoFacts

	account    ghx.Account
	accountErr error
	emails     []ghx.Email
	emailsErr  error
	mirror     ghx.Repo
	mirrorErr  error
}

type repoFacts struct {
	path      string
	missing   bool
	notGit    bool
	hasGitHub bool
}

func gather(gh ghx.GitHub) *facts {
	f := &facts{}
	f.git, f.gitErr = gitx.Git("", "--version")
	f.cfgPath, _ = config.Path()
	f.cfg, f.cfgErr = config.Load()
	f.secretPath, _ = config.SecretPath()
	_, f.secretErr = ident.Load(f.secretPath)
	f.account, f.accountErr = gh.Account()
	if f.cfgErr != nil {
		return f
	}
	for _, repo := range f.cfg.Repos {
		rf := repoFacts{path: repo}
		if _, err := os.Stat(repo); err != nil {
			rf.missing = true
		} else if hasGitHub, err := source.HasGitHubRemote(repo); err != nil {
			rf.notGit = true
		} else {
			rf.hasGitHub = hasGitHub
		}
		f.repos = append(f.repos, rf)
	}
	if f.accountErr != nil {
		return f
	}
	if !strings.EqualFold(f.cfg.Author.Email, f.account.NoreplyEmail) {
		f.emails, f.emailsErr = gh.Emails()
	}
	if f.cfg.Mirror.Repo != "" {
		f.mirror, f.mirrorErr = gh.Repo(f.cfg.Mirror.Repo)
	}
	return f
}

// diagnose runs every check. Each reports, even when it cannot run.
func diagnose(gh ghx.GitHub) []result {
	f := gather(gh)
	rs := []result{checkGit(f), checkConfig(f), checkSecret(f)}
	rs = append(rs, checkRepos(f)...)
	return append(rs,
		checkGH(f),
		checkAuthorEmail(f),
		checkMirror(f),
		checkMirrorBranch(f),
		checkPrivateContrib(f),
	)
}

func checkGit(f *facts) result {
	if f.gitErr != nil {
		return fail("git", "git not found", "install git and put it on PATH")
	}
	return ok("git", f.git)
}

func checkConfig(f *facts) result {
	// Load's missing-file error ends in the init hint; the fix line carries it.
	switch {
	case errors.Is(f.cfgErr, config.ErrNoConfig):
		return fail("config", "no config at "+f.cfgPath, "run gbridge init")
	case f.cfgErr != nil:
		return fail("config", f.cfgErr.Error(), "run gbridge init")
	}
	return ok("config", "config loaded")
}

func checkSecret(f *facts) result {
	const fix = "run gbridge init, or gbridge init --secret <phrase> to reuse another machine's secret"
	switch {
	case errors.Is(f.secretErr, ident.ErrNoSecret):
		return fail("secret", "no secret at "+f.secretPath, fix)
	case f.secretErr != nil:
		return fail("secret", f.secretErr.Error(), fix)
	}
	return ok("secret", "secret present (same recovery phrase on every machine, or work double counts)")
}

func checkRepos(f *facts) []result {
	if f.cfgErr != nil {
		return []result{skipped("repos", "config")}
	}
	if len(f.repos) == 0 {
		return []result{warn("repos", "no source repos in config, nothing to mirror")}
	}
	var rs []result
	for _, r := range f.repos {
		switch {
		case r.missing:
			rs = append(rs, fail("repos", "source repo missing: "+r.path, "remove it from config repos or restore the clone"))
		case r.notGit:
			rs = append(rs, fail("repos", "not a git repo: "+r.path, "remove it from config repos"))
		case r.hasGitHub && f.cfg.IncludeGitHubRepos:
			rs = append(rs, warn("repos", "github.com remote on an included repo, counts twice: "+r.path))
		default:
			rs = append(rs, ok("repos", "source repo "+r.path))
		}
	}
	return rs
}

func checkGH(f *facts) result {
	switch {
	case errors.Is(f.accountErr, ghx.ErrNotInstalled):
		return fail("gh", "gh not found", "install GitHub CLI and run gh auth login")
	case f.accountErr != nil:
		return fail("gh", "gh is not logged in", "run gh auth login")
	}
	return ok("gh", "GitHub account "+f.account.Login)
}

func checkAuthorEmail(f *facts) result {
	const check = "author email"
	switch {
	case f.cfgErr != nil:
		return skipped(check, "config")
	case f.accountErr != nil:
		return skipped(check, "gh")
	}
	email, u := f.cfg.Author.Email, f.account
	if strings.EqualFold(email, u.NoreplyEmail) {
		return ok(check, "author email is this account's noreply address")
	}
	if f.emailsErr != nil {
		return unverified(check, "author email "+email+" not confirmed as verified on "+u.Login,
			"https://github.com/settings/emails, or set author.email to "+u.NoreplyEmail)
	}
	for _, e := range f.emails {
		if strings.EqualFold(e.Address, email) && e.Verified {
			return ok(check, "author email verified on "+u.Login)
		}
	}
	return fail(check, "author email "+email+" is not verified on "+u.Login,
		"verify it at https://github.com/settings/emails or set author.email to "+u.NoreplyEmail)
}

func checkMirror(f *facts) result {
	const check = "mirror"
	switch {
	case f.cfgErr != nil:
		return skipped(check, "config")
	case f.cfg.Mirror.LocalOnly():
		return warn(check, "mirror is local only, nothing reaches GitHub: "+f.cfg.Mirror.Path)
	case f.cfg.Mirror.Repo == "":
		return warn(check, "mirror pushes to "+f.cfg.Mirror.Remote+", which doctor cannot check (mirror.repo is empty)")
	case f.accountErr != nil:
		return skipped(check, "gh")
	}
	repo, m, u := f.cfg.Mirror.Repo, f.mirror, f.account.Login
	switch {
	case f.mirrorErr != nil:
		return fail(check, "mirror repo "+repo+" not reachable", "gh repo create "+repo+" --private, or fix mirror.repo")
	case !strings.EqualFold(m.Owner, u):
		return fail(check, "mirror repo is owned by "+m.Owner+", not "+u, "use a repo under your own account")
	case m.Fork:
		return fail(check, "mirror repo is a fork, GitHub ignores commits to forks", "create a fresh repo")
	}
	vis := "public"
	if m.Private {
		vis = "private"
	}
	return ok(check, "mirror repo "+repo+" owned by you, not a fork, "+vis)
}

func checkMirrorBranch(f *facts) result {
	const check = "mirror branch"
	if checkMirror(f).status != statusOK {
		return skipped(check, "a usable GitHub mirror")
	}
	if b := f.mirror.DefaultBranch; b != "" && b != f.cfg.Mirror.Branch {
		return fail(check, "mirror default branch is "+b+", commits go to "+f.cfg.Mirror.Branch,
			"set mirror.branch to "+b+" or change the default branch on GitHub")
	}
	return ok(check, "commits land on the default branch")
}

func checkPrivateContrib(f *facts) result {
	const check = "private contributions"
	if checkMirror(f).status != statusOK {
		return skipped(check, "a usable GitHub mirror")
	}
	if !f.mirror.Private {
		return ok(check, "public mirror, the private contributions setting does not apply")
	}
	return unverified(check, "private contributions setting (no API exposes it)", PrivateContribSettings)
}

// Doctor checks every precondition for squares to appear. It returns the
// number of failures.
func Doctor(w io.Writer, gh ghx.GitHub) int {
	fails := 0
	for _, r := range diagnose(gh) {
		fmt.Fprintf(w, "%-12s %s\n", r.status, r.msg)
		switch r.status {
		case statusFail:
			fails++
			fmt.Fprintf(w, "             fix: %s\n", r.fix)
		case statusUnverified:
			fmt.Fprintf(w, "             check: %s\n", r.fix)
		}
	}
	fmt.Fprintln(w, "\nnote: the graph can take up to 24 hours to update after a push.")
	return fails
}
