package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ident"
	"github.com/sorfeb/greenbridge/internal/mirror"
	"github.com/sorfeb/greenbridge/internal/source"
)

// RepoStat summarizes one source repo.
type RepoStat struct {
	Path       string
	Matched    int
	Skipped    string // reason, empty when read
	ScanFolder string // scan folder the repo was found in; empty when listed in repos
	FetchErr   error  // asked to fetch and it failed; Matched counts what the repo already had
	New        bool   // found in a scan folder and not yet recorded by a sync
}

// scanDepth is how many folder levels below a scan folder repos are found.
const scanDepth = 4

// Desired is the full set of entries the mirror should contain.
type Desired struct {
	Entries []mirror.Entry // deduplicated, sorted by time
	Matched int            // commits matched before dedup
	Repos   []RepoStat
}

// PlanOptions controls how a plan is computed.
type PlanOptions struct {
	Since   time.Time // zero means all history
	Offline bool      // read the local mirror only, never contact origin
	Fetch   bool      // git fetch every source repo before reading it
}

// Plan is what the mirror should gain: the desired entries, the mirror's
// ledger, and the difference. Plan and sync both start from it; Report shows
// it, Publish writes it.
type Plan struct {
	Desired
	Ledger   int            // IDs already in the mirror
	New      []mirror.Entry // desired entries missing from the mirror, sorted by time
	FetchErr error          // fetch failed: Ledger and New are from the last successful fetch
	Orphans  int            // IDs in the mirror no source repo produces; zero with Since, which leaves older work out on purpose

	clone  mirror.Clone
	ledger *mirror.Ledger
	since  time.Time
}

// ComputePlan reads every configured repo, and every repo found in a scan
// folder, and compares the result with the mirror. A failed fetch is
// recorded in FetchErr, not returned: a preview still works offline, and
// Publish refuses to write from it.
func ComputePlan(cfg *config.Config, secret []byte, opt PlanOptions) (*Plan, error) {
	d, err := Collect(cfg, secret, opt)
	if err != nil {
		return nil, err
	}
	if err := markNew(cfg, d.Repos); err != nil {
		return nil, err
	}
	p := &Plan{Desired: *d, clone: MirrorClone(cfg), since: opt.Since}
	if opt.Offline {
		p.clone.Remote = ""
	}
	if p.ledger, err = p.clone.Open(); err != nil {
		return nil, err
	}
	p.Ledger, p.New, p.FetchErr = len(p.ledger.IDs), p.ledger.Missing(p.Entries), p.ledger.FetchErr
	p.Orphans = p.orphans()
	return p, nil
}

// orphans counts the ledger's IDs missing from desired. With since, desired
// leaves older work out on purpose, so none are counted.
func (p *Plan) orphans() int {
	if !p.since.IsZero() {
		return 0
	}
	return p.ledger.Orphans(p.Entries)
}

// WriteOrphans writes the orphan line, or nothing when there are none. Plan
// and sync both print it.
func WriteOrphans(w io.Writer, n int) {
	if n > 0 {
		fmt.Fprintf(w, "%d in mirror not found in source repos on this machine\n", n)
	}
}

// markNew flags the repos found in scan folders that no sync has recorded.
func markNew(cfg *config.Config, repos []RepoStat) error {
	if len(cfg.ScanFolders) == 0 {
		return nil
	}
	seen, err := config.LoadSeen()
	if err != nil {
		return err
	}
	for i := range repos {
		r := &repos[i]
		r.New = r.ScanFolder != "" && r.Skipped == "" && !seen[r.Path]
	}
	return nil
}

// NewRepos returns the repos found in scan folders that no sync has
// recorded yet.
func (p *Plan) NewRepos() []RepoStat {
	var out []RepoStat
	for _, r := range p.Repos {
		if r.New {
			out = append(out, r)
		}
	}
	return out
}

// FetchFailed returns the source repos whose fetch failed. Each was read as
// it was.
func (p *Plan) FetchFailed() []RepoStat { return fetchFailed(p.Repos) }

func fetchFailed(repos []RepoStat) []RepoStat {
	var out []RepoStat
	for _, r := range repos {
		if r.FetchErr != nil {
			out = append(out, r)
		}
	}
	return out
}

// WriteFetchFailed writes one line per source repo whose fetch failed. Plan
// and sync both print it.
func WriteFetchFailed(w io.Writer, repos []RepoStat) {
	for _, r := range repos {
		fmt.Fprintf(w, "  fetch failed %s: %v\n", r.Path, r.FetchErr)
	}
}

// WriteNewRepos writes one line per repo a scan folder contributed for the
// first time. Plan and sync both print it.
func WriteNewRepos(w io.Writer, repos []RepoStat) {
	for _, r := range repos {
		fmt.Fprintf(w, "  new   %s (from scan folder %s)\n", r.Path, r.ScanFolder)
	}
}

// Collect reads every configured repo and every repo found in a scan folder,
// and computes mirror entries. A found repo counts only when it has commits
// by a configured email and no github.com remote (unless cfg includes
// those); any other found repo is left out of Repos without a word, since a
// scan folder holds many.
func Collect(cfg *config.Config, secret []byte, opt PlanOptions) (*Desired, error) {
	repos, missing := sources(cfg)
	found := inspectAll(cfg, repos, opt.Since, opt.Fetch)
	d := &Desired{}
	seen := map[string]bool{}
	for i, r := range repos {
		in, inFolder := found[i], r.scanFolder != ""
		st := RepoStat{Path: r.path, ScanFolder: r.scanFolder, FetchErr: in.fetchErr}
		switch {
		case in.unreadable:
			st.Skipped = "not a readable git repo"
		case in.hosted && inFolder:
			continue
		case in.hosted:
			st.Skipped = "has a github.com remote (GitHub already counts it)"
		case in.err != nil && inFolder:
			st.Skipped = in.err.Error()
		case in.err != nil:
			return nil, in.err
		case len(in.acts) == 0 && inFolder:
			continue
		default:
			st.Matched = len(in.acts)
		}
		d.Repos = append(d.Repos, st)
		d.Matched += st.Matched
		for _, a := range in.acts {
			id := ident.ID(secret, cfg.Identity, a.Email, a.SHA, a.Time)
			if seen[id] {
				continue
			}
			seen[id] = true
			d.Entries = append(d.Entries, mirror.Entry{ID: id, Time: ident.Stamp(a.Time, id, cfg.TimePolicy)})
		}
	}
	for _, dir := range missing {
		d.Repos = append(d.Repos, RepoStat{Path: dir, ScanFolder: dir, Skipped: "scan folder not found"})
	}
	sort.SliceStable(d.Entries, func(i, j int) bool { return d.Entries[i].Time.Before(d.Entries[j].Time) })
	return d, nil
}

// sourceRepo is one source repo to read: listed in repos (scanFolder empty),
// or found in scanFolder.
type sourceRepo struct{ path, scanFolder string }

// sources lists cfg's repos, then the repos found in each scan folder, once
// each by cleaned path. Finding repos only looks for .git entries, so git
// never runs on a folder that is not a repo. Scan folders that do not exist
// come back apart.
func sources(cfg *config.Config) (repos []sourceRepo, missing []string) {
	listed := map[string]bool{}
	add := func(path, scanFolder string) {
		path = filepath.Clean(path)
		if !listed[path] {
			listed[path] = true
			repos = append(repos, sourceRepo{path, scanFolder})
		}
	}
	for _, r := range cfg.Repos {
		add(r, "")
	}
	for _, dir := range cfg.ScanFolders {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			missing = append(missing, dir)
			continue
		}
		for _, r := range findRepos(dir) {
			add(r.path, dir)
		}
	}
	return repos, missing
}

// findRepos returns the repos in scan folder dir.
func findRepos(dir string) []sourceRepo {
	var out []sourceRepo
	for _, r := range source.FindRepos(dir, scanDepth) {
		out = append(out, sourceRepo{filepath.Clean(r), dir})
	}
	return out
}

// inspection is what one source repo holds for cfg, from one git log.
type inspection struct {
	unreadable bool              // err says why
	gitHub     bool              // has a github.com remote
	hosted     bool              // gitHub, and cfg leaves those out: not read
	fetchErr   error             // asked to fetch and it failed
	acts       []source.Activity // by cfg's emails, on or after since
	authors    []source.Author   // every author on or after since
	err        error
}

// inspectAll inspects repos in parallel, since each inspection mostly waits
// on git. Results keep the order of repos.
func inspectAll(cfg *config.Config, repos []sourceRepo, since time.Time, fetch bool) []inspection {
	out := make([]inspection, len(repos))
	slots := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			out[i] = inspect(cfg, r.path, since, fetch)
		})
	}
	wg.Wait()
	return out
}

// inspect reads repo once and keeps both views of its history, so the full
// history is dropped before the next repo is read.
func inspect(cfg *config.Config, repo string, since time.Time, fetch bool) inspection {
	in, err := source.Inspect(repo, source.InspectOptions{SkipGitHub: !cfg.IncludeGitHubRepos, Fetch: fetch})
	switch {
	case errors.Is(err, source.ErrUnreadable):
		return inspection{unreadable: true, err: err}
	case err != nil:
		return inspection{gitHub: in.GitHubRemote, fetchErr: in.FetchErr, err: err}
	case in.History == nil:
		return inspection{gitHub: true, hosted: true}
	}
	return inspection{
		gitHub:   in.GitHubRemote,
		fetchErr: in.FetchErr,
		acts:     in.History.Activity(cfg.Emails, since),
		authors:  in.History.Authors(since),
	}
}

// MirrorClone builds the mirror clone handle from the mirror settings.
func MirrorClone(cfg *config.Config) mirror.Clone {
	return mirror.Clone{Path: cfg.Mirror.Path, Remote: cfg.Mirror.Remote, Branch: cfg.Mirror.Branch}
}
