package app

import (
	"errors"
	"strings"

	"github.com/sorfeb/greenbridge/internal/ghx"
)

// PrivateContribSettings is where the private contributions toggle lives.
const PrivateContribSettings = "https://github.com/settings/profile (Contributions & activity: Include private contributions on my profile)"

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
	email, u := f.cfg.MirrorAuthor.Email, f.account
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
