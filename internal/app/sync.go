// Package app wires sources, identity, and the mirror into gbridge commands.
package app

import (
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

// SyncResult reports what a sync did.
type SyncResult struct {
	Desired  int
	Ledger   int
	Added    int
	Pushed   bool
	Orphans  int        // IDs in the mirror no source repo produces; never deleted
	NewRepos []RepoStat // repos a scan folder contributed for the first time
}

// Sync brings the mirror up to date: ComputePlan, then Publish. Unless it is
// a dry run, it then records the new repos found in scan folders, so the
// next run no longer calls them new.
func Sync(cfg *config.Config, secret []byte, opt SyncOptions, log io.Writer) (*SyncResult, error) {
	p, err := ComputePlan(cfg, secret, PlanOptions{Since: opt.Since})
	if err != nil {
		return nil, err
	}
	res, err := Publish(cfg, p, mirror.PublishOptions{DryRun: opt.DryRun, NoPush: opt.NoPush}, log)
	if err != nil {
		return nil, err
	}
	res.NewRepos = p.NewRepos()
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

// Publish writes the plan's new entries to the mirror clone and pushes them,
// through the mirror package's append-and-publish protocol: it refuses a plan
// whose fetch failed, and retries a push another machine raced.
func Publish(cfg *config.Config, p *Plan, opt mirror.PublishOptions, log io.Writer) (*SyncResult, error) {
	a := mirror.Author{Name: cfg.MirrorAuthor.Name, Email: cfg.MirrorAuthor.Email}
	pub, err := p.clone.Publish(p.ledger, p.Entries, a, opt, log)
	if err != nil {
		return nil, err
	}
	// A retried push rereads the ledger, so count orphans after Publish.
	return &SyncResult{Desired: len(p.Entries), Ledger: pub.Ledger, Added: pub.Added, Pushed: pub.Pushed, Orphans: p.orphans()}, nil
}
