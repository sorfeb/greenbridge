package app

import (
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

type report struct {
	w     io.Writer
	fails int
}

func (r *report) ok(msg string)   { fmt.Fprintf(r.w, "[ok]         %s\n", msg) }
func (r *report) warn(msg string) { fmt.Fprintf(r.w, "[warn]       %s\n", msg) }
func (r *report) unverified(msg, fix string) {
	fmt.Fprintf(r.w, "[unverified] %s\n             check: %s\n", msg, fix)
}

func (r *report) fail(msg, fix string) {
	r.fails++
	fmt.Fprintf(r.w, "[FAIL]       %s\n             fix: %s\n", msg, fix)
}

// Doctor checks every precondition for squares to appear. It returns the
// number of failures.
func Doctor(w io.Writer) int {
	r := &report{w: w}

	if v, err := gitx.Git("", "--version"); err != nil {
		r.fail("git not found", "install git and put it on PATH")
	} else {
		r.ok(v)
	}

	cfg, err := config.Load()
	if err != nil {
		r.fail(err.Error(), "run gb init")
		return r.fails
	}
	r.ok("config loaded")

	sp, _ := config.SecretPath()
	if _, err := ident.Load(sp); err != nil {
		r.fail(err.Error(), "run gb init, or gb init --secret <phrase> to reuse another machine's secret")
	} else {
		r.ok("secret present (same recovery phrase on every machine, or work double counts)")
	}

	for _, repo := range cfg.Repos {
		if _, err := os.Stat(repo); err != nil {
			r.fail("source repo missing: "+repo, "remove it from config repos or restore the clone")
			continue
		}
		gh, err := source.HasGitHubRemote(repo)
		if err != nil {
			r.fail("not a git repo: "+repo, "remove it from config repos")
		} else if gh && cfg.IncludeGitHubRepos {
			r.warn("github.com remote on an included repo, counts twice: " + repo)
		}
	}

	if !ghx.Available() {
		r.fail("gh not found", "install GitHub CLI and run gh auth login")
		return r.fails
	}
	u, err := ghx.CurrentUser()
	if err != nil {
		r.fail("gh is not logged in", "run gh auth login")
		return r.fails
	}
	r.ok("GitHub account " + u.Login)

	if strings.EqualFold(cfg.Author.Email, u.NoreplyEmail()) {
		r.ok("author email is this account's noreply address")
	} else if es, err := ghx.Emails(); err != nil {
		r.unverified("author email "+cfg.Author.Email+" not confirmed as verified on "+u.Login,
			"https://github.com/settings/emails, or set author.email to "+u.NoreplyEmail())
	} else {
		verified := false
		for _, e := range es {
			if strings.EqualFold(e.Email, cfg.Author.Email) && e.Verified {
				verified = true
			}
		}
		if verified {
			r.ok("author email verified on " + u.Login)
		} else {
			r.fail("author email "+cfg.Author.Email+" is not verified on "+u.Login,
				"verify it at https://github.com/settings/emails or set author.email to "+u.NoreplyEmail())
		}
	}

	if cfg.Mirror.Repo == "" {
		r.warn("mirror is local only, nothing reaches GitHub: " + cfg.Mirror.Path)
	} else {
		repo, err := ghx.ViewRepo(cfg.Mirror.Repo)
		switch {
		case err != nil:
			r.fail("mirror repo "+cfg.Mirror.Repo+" not reachable", "gh repo create "+cfg.Mirror.Repo+" --private, or fix mirror.repo")
		case !strings.EqualFold(repo.Owner.Login, u.Login):
			r.fail("mirror repo is owned by "+repo.Owner.Login+", not "+u.Login, "use a repo under your own account")
		case repo.IsFork:
			r.fail("mirror repo is a fork, GitHub ignores commits to forks", "create a fresh repo")
		default:
			r.ok("mirror repo " + cfg.Mirror.Repo + " owned by you, not a fork, " + strings.ToLower(repo.Visibility))
			if b := repo.DefaultBranchRef.Name; b != "" && b != cfg.Mirror.Branch {
				r.fail("mirror default branch is "+b+", commits go to "+cfg.Mirror.Branch, "set mirror.branch to "+b+" or change the default branch on GitHub")
			} else {
				r.ok("commits land on the default branch")
			}
			if strings.EqualFold(repo.Visibility, "private") {
				r.unverified("private contributions setting (no API exposes it)", PrivateContribSettings)
			}
		}
	}

	fmt.Fprintln(w, "\nnote: the graph can take up to 24 hours to update after a push.")
	return r.fails
}
