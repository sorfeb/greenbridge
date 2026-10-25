// Package app wires sources, identity, and the mirror into gbridge commands.
package app

import (
	"errors"
	"fmt"
	"io"
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

// SyncOptions controls a sync run.
type SyncOptions struct {
	Since  time.Time
	DryRun bool // compute only, write nothing
	NoPush bool // write local mirror, skip push
}

// SyncResult reports what a sync did.
type SyncResult struct {
	Desired  int
	Existing int
	Added    int
	Pushed   bool
}

// Sync brings the mirror up to date. On a push race it refetches, recomputes
// the difference from the new remote tip, and retries.
func Sync(cfg *config.Config, secret []byte, opt SyncOptions, log io.Writer) (*SyncResult, error) {
	d, err := Collect(cfg, secret, opt.Since)
	if err != nil {
		return nil, err
	}
	m := MirrorOf(cfg)
	if err := m.Ensure(); err != nil {
		return nil, err
	}
	for attempt := range 3 {
		tip, err := m.Fetch()
		if err != nil {
			return nil, err
		}
		ledger, err := m.Ledger(tip)
		if err != nil {
			return nil, err
		}
		todo := Diff(d.Entries, ledger)
		res := &SyncResult{Desired: len(d.Entries), Existing: len(ledger), Added: len(todo)}
		if len(todo) == 0 || opt.DryRun {
			return res, nil
		}
		if _, err := m.Emit(tip, todo, mirror.Author{Name: cfg.Author.Name, Email: cfg.Author.Email}); err != nil {
			return nil, err
		}
		if m.Remote == "" || opt.NoPush {
			return res, nil
		}
		if err := m.Push(); err != nil {
			fmt.Fprintf(log, "push rejected, refetching (attempt %d): %v\n", attempt+1, err)
			continue
		}
		res.Pushed = true
		return res, nil
	}
	return nil, errors.New("push kept failing after 3 attempts; run gbridge doctor")
}
