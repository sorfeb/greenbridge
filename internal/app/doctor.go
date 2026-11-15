package app

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

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
	scans []scanFacts

	account    ghx.Account
	accountErr error
	emails     []ghx.Email
	emailsErr  error
	mirror     ghx.Repo
	mirrorErr  error
}

// scanFacts is one scan folder: whether it exists, and how many repos it
// holds. Doctor only lists scan folders; plan and sync judge their repos.
type scanFacts struct {
	path    string
	missing bool
	repos   int
}

type repoFacts struct {
	path      string
	missing   bool
	notGit    bool
	hasGitHub bool

	// authors made commits in the last year on the refs sync reads. userName
	// is git's user.name in the repo, empty when unset.
	authors  []source.Author
	userName string
}

// emailWindow is how far back doctor looks for commits under unlisted
// emails: the year the contribution graph shows.
const emailWindow = "the last year"

func emailSince() time.Time { return time.Now().AddDate(-1, 0, 0) }

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
			if !hasGitHub || f.cfg.IncludeGitHubRepos {
				rf.authors, _ = source.Authors(repo, emailSince())
				rf.userName, _ = gitx.Git(repo, "config", "user.name")
			}
		}
		f.repos = append(f.repos, rf)
	}
	for _, dir := range f.cfg.Scan {
		sf := scanFacts{path: dir}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			sf.missing = true
		} else {
			sf.repos = len(findRepos(dir))
		}
		f.scans = append(f.scans, sf)
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
	rs = append(rs, checkEmails(f)...)
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
	if len(f.repos) == 0 && len(f.scans) == 0 {
		return []result{warn("repos", "no source repos or scan folders in config, nothing to mirror")}
	}
	var rs []result
	for _, r := range f.repos {
		switch {
		case r.missing:
			rs = append(rs, fail("repos", "source repo missing: "+r.path, "gbridge remove repo "+r.path+", or restore the clone"))
		case r.notGit:
			rs = append(rs, fail("repos", "not a git repo: "+r.path, "gbridge remove repo "+r.path))
		case r.hasGitHub && f.cfg.IncludeGitHubRepos:
			rs = append(rs, warn("repos", "github.com remote on an included repo, counts twice: "+r.path))
		default:
			rs = append(rs, ok("repos", "source repo "+r.path))
		}
	}
	for _, s := range f.scans {
		if s.missing {
			rs = append(rs, fail("repos", "scan folder missing: "+s.path, "gbridge remove scan "+s.path+", or restore the folder"))
		} else {
			rs = append(rs, ok("repos", fmt.Sprintf("scan folder %s (%d git repos)", s.path, s.repos)))
		}
	}
	return rs
}

// checkEmails warns about commits under an email missing from config. In a
// shared repo most unlisted authors are coworkers, so only commits under the
// user's own author name count: a name on commits by a configured email, or
// git's user.name. An email that also carries other names is reported but
// never suggested. Only the email and counts are printed.
func checkEmails(f *facts) []result {
	const check = "emails"
	if f.cfgErr != nil {
		return []result{skipped(check, "config")}
	}
	listed := map[string]bool{}
	for _, e := range f.cfg.Emails {
		listed[strings.ToLower(strings.TrimSpace(e))] = true
	}
	names := map[string]bool{}
	for _, r := range f.repos {
		if n := strings.ToLower(strings.TrimSpace(r.userName)); n != "" {
			names[n] = true
		}
		for _, a := range r.authors {
			if listed[a.Email] && a.Name != "" {
				names[strings.ToLower(a.Name)] = true
			}
		}
	}
	// others counts commits under other names on the same unlisted email: a
	// coworker's address the user once committed with. Adding it would mirror
	// the coworker's work as the user's, so such an email gets no fix.
	counts, others := map[string]int{}, map[string]int{}
	for _, r := range f.repos {
		for _, a := range r.authors {
			switch {
			case listed[a.Email]:
			case names[strings.ToLower(a.Name)]:
				counts[a.Email] += a.Commits
			default:
				others[a.Email] += a.Commits
			}
		}
	}
	if len(counts) == 0 {
		return []result{ok(check, "no commits in "+emailWindow+" under your author name and an unlisted email")}
	}
	emails := slices.Collect(maps.Keys(counts))
	slices.SortFunc(emails, func(a, b string) int { return cmp.Or(counts[b]-counts[a], strings.Compare(a, b)) })
	rs := make([]result, 0, len(emails))
	for _, e := range emails {
		mine := fmt.Sprintf("%s in %s under your author name use %s", plural(counts[e], "commit"), emailWindow, e)
		if others[e] > 0 {
			rs = append(rs, ok(check, fmt.Sprintf("%s, which also has %s under other author names: left out",
				mine, plural(others[e], "commit"))))
			continue
		}
		rs = append(rs, result{check, statusWarn, mine + ", which is not in config emails", "gbridge add email " + e})
	}
	return rs
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
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
