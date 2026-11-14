package app

import (
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
	Path    string
	Matched int
	Skipped string // reason, empty when scanned
	Scan    string // scan folder the repo was found in; empty when listed in repos
	New     bool   // found in a scan folder and not yet recorded by a sync
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
}

// Plan is what the mirror should gain: the desired entries, the mirror's
// ledger, and the difference. Plan and sync both start from it; Report shows
// it, Publish writes it.
type Plan struct {
	Desired
	Ledger   int            // IDs already in the mirror
	New      []mirror.Entry // desired entries missing from the mirror, sorted by time
	FetchErr error          // fetch failed: Ledger and New are from the last successful fetch

	mirror mirror.Mirror
	tip    string // mirror commit New builds on
}

// ComputePlan scans every configured repo, and every repo found in a scan
// folder, and compares the result with the mirror. A failed fetch is
// recorded in FetchErr, not returned: a preview still works offline, and
// Publish refuses to write from it.
func ComputePlan(cfg *config.Config, secret []byte, opt PlanOptions) (*Plan, error) {
	d, err := Collect(cfg, secret, opt.Since)
	if err != nil {
		return nil, err
	}
	if err := markNew(cfg, d.Repos); err != nil {
		return nil, err
	}
	p := &Plan{Desired: *d, mirror: MirrorOf(cfg)}
	if opt.Offline {
		p.mirror.Remote = ""
	}
	if err := p.mirror.Ensure(); err != nil {
		return nil, err
	}
	return p, p.read()
}

// markNew flags the repos found in scan folders that no sync has recorded.
func markNew(cfg *config.Config, repos []RepoStat) error {
	if len(cfg.Scan) == 0 {
		return nil
	}
	seen, err := config.LoadSeen()
	if err != nil {
		return err
	}
	for i := range repos {
		r := &repos[i]
		r.New = r.Scan != "" && r.Skipped == "" && !seen[r.Path]
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

// WriteNewRepos writes one line per repo a scan folder contributed for the
// first time. Plan and sync both print it.
func WriteNewRepos(w io.Writer, repos []RepoStat) {
	for _, r := range repos {
		fmt.Fprintf(w, "  new   %s (from scan folder %s)\n", r.Path, r.Scan)
	}
}

// read fills the mirror half of p from the mirror's current tip, falling back
// to the last fetched tip when the fetch fails.
func (p *Plan) read() error {
	tip, err := p.mirror.Fetch()
	p.FetchErr = err
	if err != nil {
		tip = p.mirror.Last()
	}
	ledger, err := p.mirror.Ledger(tip)
	if err != nil {
		return err
	}
	p.tip, p.Ledger, p.New = tip, len(ledger), Diff(p.Entries, ledger)
	return nil
}

// Collect scans every configured repo and every repo found in a scan folder,
// and computes mirror entries. A found repo counts only when it has commits
// by a configured email and no github.com remote (unless cfg includes
// those); any other found repo is left out of Repos without a word, since a
// scan folder holds many.
func Collect(cfg *config.Config, secret []byte, since time.Time) (*Desired, error) {
	repos, missing := sources(cfg)
	found := inspectAll(cfg, repos, since)
	d := &Desired{}
	seen := map[string]bool{}
	for i, r := range repos {
		in, scanned := found[i], r.scan != ""
		st := RepoStat{Path: r.path, Scan: r.scan}
		switch {
		case in.unreadable:
			st.Skipped = "not a readable git repo"
		case in.hosted && scanned:
			continue
		case in.hosted:
			st.Skipped = "has a github.com remote (GitHub already counts it)"
		case in.err != nil && scanned:
			st.Skipped = in.err.Error()
		case in.err != nil:
			return nil, in.err
		case len(in.acts) == 0 && scanned:
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
		d.Repos = append(d.Repos, RepoStat{Path: dir, Scan: dir, Skipped: "scan folder not found"})
	}
	sort.SliceStable(d.Entries, func(i, j int) bool { return d.Entries[i].Time.Before(d.Entries[j].Time) })
	return d, nil
}

// sourceRepo is one repo to read: listed in repos (scan empty), or found in
// scan folder scan.
type sourceRepo struct{ path, scan string }

// sources lists cfg's repos, then the repos found in each scan folder, once
// each by cleaned path. Finding repos only looks for .git entries, so git
// never runs on a folder that is not a repo. Scan folders that do not exist
// come back apart.
func sources(cfg *config.Config) (repos []sourceRepo, missing []string) {
	listed := map[string]bool{}
	add := func(path, scan string) {
		path = filepath.Clean(path)
		if !listed[path] {
			listed[path] = true
			repos = append(repos, sourceRepo{path, scan})
		}
	}
	for _, r := range cfg.Repos {
		add(r, "")
	}
	for _, dir := range cfg.Scan {
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

// inspection is what one repo holds for cfg.
type inspection struct {
	unreadable bool // err says why
	hosted     bool // has a github.com remote, and cfg leaves those out
	acts       []source.Activity
	err        error
}

// inspectAll inspects repos in parallel, since each inspection mostly waits
// on git. Results keep the order of repos.
func inspectAll(cfg *config.Config, repos []sourceRepo, since time.Time) []inspection {
	out := make([]inspection, len(repos))
	slots := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			out[i] = inspect(cfg, r.path, since)
		})
	}
	wg.Wait()
	return out
}

// inspect checks the remote first, so a repo cfg leaves out costs one git
// run instead of two.
func inspect(cfg *config.Config, repo string, since time.Time) inspection {
	gh, err := source.HasGitHubRemote(repo)
	if err != nil {
		return inspection{unreadable: true, err: err}
	}
	if gh && !cfg.IncludeGitHubRepos {
		return inspection{hosted: true}
	}
	acts, err := source.Scan(repo, cfg.Emails, since)
	return inspection{acts: acts, err: err}
}

// MirrorOf builds the mirror handle from config.
func MirrorOf(cfg *config.Config) mirror.Mirror {
	return mirror.Mirror{Path: cfg.Mirror.Path, Remote: cfg.Mirror.Remote, Branch: cfg.Mirror.Branch}
}

// Diff returns entries whose IDs are not in the ledger.
func Diff(entries []mirror.Entry, ledger map[string]bool) []mirror.Entry {
	var out []mirror.Entry
	for _, e := range entries {
		if !ledger[e.ID] {
			out = append(out, e)
		}
	}
	return out
}
