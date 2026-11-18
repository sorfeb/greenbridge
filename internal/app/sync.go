// Package app wires sources, identity, and the mirror into gbridge commands.
package app

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/mirror"
)

// SyncOptions controls a sync run.
type SyncOptions struct {
	Since  time.Time
	DryRun bool // compute only, write nothing
	NoPush bool // write local mirror, skip push
	Fetch  bool // git fetch every source repo before reading it
}

// PublishOptions controls how a plan is written.
type PublishOptions struct {
	DryRun bool // compute only, write nothing
	NoPush bool // write local mirror, skip push
}

// SyncResult reports what a sync did.
type SyncResult struct {
	Desired     int
	Ledger      int
	Added       int
	Pushed      bool
	NewRepos    []RepoStat // repos a scan folder contributed for the first time
	FetchFailed []RepoStat // source repos whose fetch failed, read as they were
}

// Sync brings the mirror up to date: ComputePlan, then Publish. Unless it is
// a dry run, it then records the new repos found in scan folders, so the
// next run no longer calls them new.
func Sync(cfg *config.Config, secret []byte, opt SyncOptions, log io.Writer) (*SyncResult, error) {
	p, err := ComputePlan(cfg, secret, PlanOptions{Since: opt.Since, Fetch: opt.Fetch})
	if err != nil {
		return nil, err
	}
	res, err := Publish(cfg, p, PublishOptions{DryRun: opt.DryRun, NoPush: opt.NoPush}, log)
	if err != nil {
		return nil, err
	}
	res.NewRepos, res.FetchFailed = p.NewRepos(), p.FetchFailed()
	if opt.DryRun {
		return res, nil
	}
	var paths []string
	for _, r := range res.NewRepos {
		paths = append(paths, r.Path)
	}
	// The mirror is already written; a lost record only repeats the notice.
	if err := config.AddSeen(paths); err != nil {
		fmt.Fprintf(log, "could not record repos found in scan folders: %v\n", err)
	}
	return res, nil
}

// Publish writes the plan's new entries to the mirror and pushes them. It
// refuses a plan whose fetch failed: entries built on a stale tip either
// duplicate what another machine pushed or get rejected. On a push race it
// refetches, recomputes p against the new remote tip, and retries.
func Publish(cfg *config.Config, p *Plan, opt PublishOptions, log io.Writer) (*SyncResult, error) {
	for attempt := range 3 {
		if attempt > 0 {
			if err := p.read(); err != nil {
				return nil, err
			}
		}
		if p.FetchErr != nil {
			return nil, fmt.Errorf("mirror fetch failed, nothing written: %w", p.FetchErr)
		}
		res := &SyncResult{Desired: len(p.Entries), Ledger: p.Ledger, Added: len(p.New)}
		if len(p.New) == 0 || opt.DryRun {
			return res, nil
		}
		if _, err := p.clone.Emit(p.tip, p.New, mirror.Author{Name: cfg.MirrorAuthor.Name, Email: cfg.MirrorAuthor.Email}); err != nil {
			return nil, err
		}
		if p.clone.Remote == "" || opt.NoPush {
			return res, nil
		}
		if err := p.clone.Push(); err != nil {
			fmt.Fprintf(log, "push rejected, refetching (attempt %d): %v\n", attempt+1, err)
			continue
		}
		res.Pushed = true
		return res, nil
	}
	return nil, errors.New("push kept failing after 3 attempts; run gbridge doctor")
}
