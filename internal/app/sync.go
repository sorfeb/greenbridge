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
}

// PublishOptions controls how a plan is written.
type PublishOptions struct {
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

// Sync brings the mirror up to date: ComputePlan, then Publish.
func Sync(cfg *config.Config, secret []byte, opt SyncOptions, log io.Writer) (*SyncResult, error) {
	p, err := ComputePlan(cfg, secret, PlanOptions{Since: opt.Since})
	if err != nil {
		return nil, err
	}
	return Publish(cfg, p, PublishOptions{DryRun: opt.DryRun, NoPush: opt.NoPush}, log)
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
		res := &SyncResult{Desired: len(p.Entries), Existing: p.Ledger, Added: len(p.New)}
		if len(p.New) == 0 || opt.DryRun {
			return res, nil
		}
		if _, err := p.mirror.Emit(p.tip, p.New, mirror.Author{Name: cfg.Author.Name, Email: cfg.Author.Email}); err != nil {
			return nil, err
		}
		if p.mirror.Remote == "" || opt.NoPush {
			return res, nil
		}
		if err := p.mirror.Push(); err != nil {
			fmt.Fprintf(log, "push rejected, refetching (attempt %d): %v\n", attempt+1, err)
			continue
		}
		res.Pushed = true
		return res, nil
	}
	return nil, errors.New("push kept failing after 3 attempts; run gbridge doctor")
}
