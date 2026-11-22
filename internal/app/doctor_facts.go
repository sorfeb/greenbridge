package app

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/gitx"
	"github.com/sorfeb/greenbridge/internal/ident"
	"github.com/sorfeb/greenbridge/internal/source"
)

// facts is everything doctor learns from the machine and GitHub, gathered
// once before any check runs.
type facts struct {
	online bool

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

	// remotes compares origin and tracked remotes with the repo. Only for
	// repos sync reads.
	remotes []source.Remote
}

// remoteTimeout bounds how long doctor waits for every remote together,
// counted from the start. A remote that answers later is reported as not
// checked: a slow forge is not a broken remote.
const remoteTimeout = 15 * time.Second

// remoteWorkers is how many repos list their remotes at once.
const remoteWorkers = 8

// emailWindow is how far back doctor looks for commits under unlisted
// emails: the year the contribution graph shows.
const emailWindow = "the last year"

func emailSince() time.Time { return time.Now().AddDate(-1, 0, 0) }

// gatherRepos inspects cfg's source repos the way plan does: one git log
// each, in parallel.
func gatherRepos(cfg *config.Config) []repoFacts {
	rfs := make([]repoFacts, len(cfg.Repos))
	var present []sourceRepo
	var at []int
	for i, repo := range cfg.Repos {
		rfs[i].path = repo
		if _, err := os.Stat(repo); err != nil {
			rfs[i].missing = true
			continue
		}
		present, at = append(present, sourceRepo{path: repo}), append(at, i)
	}
	for j, in := range inspectAll(cfg, present, emailSince(), false) {
		rf := &rfs[at[j]]
		rf.notGit, rf.hasGitHub, rf.authors = in.unreadable, in.gitHub, in.authors
		if !in.unreadable && !in.hosted {
			rf.userName, _ = gitx.Git(rf.path, "config", "user.name")
		}
	}
	return rfs
}

func gather(gh ghx.GitHub, opt DoctorOptions) *facts {
	f := &facts{online: opt.Online}
	f.git, f.gitErr = gitx.Git("", "--version")
	f.cfgPath, _ = config.Path()
	f.cfg, f.cfgErr = config.Load()
	f.secretPath, _ = config.SecretPath()
	_, f.secretErr = ident.Load(f.secretPath)
	if f.cfgErr != nil {
		f.account, f.accountErr = gh.Account()
		return f
	}
	f.repos = gatherRepos(f.cfg)
	if f.online {
		defer gatherRemotes(f)()
	}
	for _, dir := range f.cfg.ScanFolders {
		sf := scanFacts{path: dir}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			sf.missing = true
		} else {
			sf.repos = len(findRepos(dir))
		}
		f.scans = append(f.scans, sf)
	}
	f.account, f.accountErr = gh.Account()
	if f.accountErr != nil {
		return f
	}
	if !strings.EqualFold(f.cfg.MirrorAuthor.Email, f.account.NoreplyEmail) {
		f.emails, f.emailsErr = gh.Emails()
	}
	if f.cfg.Mirror.Repo != "" {
		f.mirror, f.mirrorErr = gh.Repo(f.cfg.Mirror.Repo)
	}
	return f
}

// gatherRemotes compares the tracked remotes of every source repo sync reads
// with the repo, in the background, and returns the wait for them. Doctor
// asks GitHub meanwhile.
func gatherRemotes(f *facts) (wait func()) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	var wg sync.WaitGroup
	sem := make(chan struct{}, remoteWorkers)
	for i := range f.repos {
		r := &f.repos[i]
		if r.missing || r.notGit || (r.hasGitHub && !f.cfg.IncludeGitHubRepos) {
			continue
		}
		wg.Go(func() { r.remotes = listRemotes(ctx, r.path, sem) })
	}
	return func() {
		wg.Wait()
		cancel()
	}
}

// listRemotes compares repo's tracked remotes with the repo once a slot in
// sem is free. A remote still unlisted when ctx ends carries its error.
func listRemotes(ctx context.Context, repo string, sem chan struct{}) []source.Remote {
	sem <- struct{}{}
	defer func() { <-sem }()
	// A repo git can't read was already judged by checkRepos.
	rs, _ := source.CheckRemotes(ctx, repo)
	return rs
}
