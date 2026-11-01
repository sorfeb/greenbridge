package app

import (
	"sort"
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
}

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

// ComputePlan scans every configured repo and compares the result with the
// mirror. A failed fetch is recorded in FetchErr, not returned: a preview
// still works offline, and Publish refuses to write from it.
func ComputePlan(cfg *config.Config, secret []byte, opt PlanOptions) (*Plan, error) {
	d, err := Collect(cfg, secret, opt.Since)
	if err != nil {
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

// Collect scans every configured repo and computes mirror entries.
func Collect(cfg *config.Config, secret []byte, since time.Time) (*Desired, error) {
	d := &Desired{}
	seen := map[string]bool{}
	for _, repo := range cfg.Repos {
		st := RepoStat{Path: repo}
		gh, err := source.HasGitHubRemote(repo)
		if err != nil {
			st.Skipped = "not a readable git repo"
			d.Repos = append(d.Repos, st)
			continue
		}
		if gh && !cfg.IncludeGitHubRepos {
			st.Skipped = "has a github.com remote (GitHub already counts it)"
			d.Repos = append(d.Repos, st)
			continue
		}
		acts, err := source.Scan(repo, cfg.Emails, since)
		if err != nil {
			return nil, err
		}
		st.Matched = len(acts)
		d.Matched += len(acts)
		d.Repos = append(d.Repos, st)
		for _, a := range acts {
			id := ident.ID(secret, cfg.Identity, a.Email, a.SHA, a.Time)
			if seen[id] {
				continue
			}
			seen[id] = true
			d.Entries = append(d.Entries, mirror.Entry{ID: id, Time: ident.Stamp(a.Time, id, cfg.TimePolicy)})
		}
	}
	sort.SliceStable(d.Entries, func(i, j int) bool { return d.Entries[i].Time.Before(d.Entries[j].Time) })
	return d, nil
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
