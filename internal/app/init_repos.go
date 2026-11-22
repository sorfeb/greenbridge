package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/gitx"
)

// askEmails sets the work emails: given, else asked with the current ones
// (or git's user.email) as the default.
func askEmails(p *prompter, cfg *config.Config, given []string) error {
	emails := given
	if len(emails) == 0 {
		def := strings.Join(cfg.Emails, ", ")
		if def == "" {
			def, _ = gitx.Git("", "config", "--global", "user.email")
		}
		emails = splitList(p.ask("Work emails to match, comma separated", def))
	}
	if len(emails) == 0 {
		return errors.New("at least one email is required")
	}
	cfg.Emails = emails
	return nil
}

// askRepos adds the given paths and scan folders to cfg (folders asked for
// when both are empty), shows which repos count, and returns the repos the
// scan folders hold now. A named repo is kept without commits by cfg's
// emails; a found one needs them. A scan folder is kept even when it holds
// none yet, since plan and sync search it again every run.
func askRepos(p *prompter, cfg *config.Config, paths, scan []string) (found []string, err error) {
	if len(paths) == 0 && len(scan) == 0 {
		scan = splitList(p.ask("Folders to scan for work repos, comma separated", ""))
	}
	var dirs []string
	var repos []sourceRepo
	for _, d := range scan {
		d = absPath(d)
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			fmt.Fprintf(p.out, "  skip  %s (not a folder)\n", d)
			continue
		}
		if !slices.Contains(dirs, d) {
			dirs, repos = append(dirs, d), append(repos, findRepos(d)...)
		}
	}
	added, found, _ := pickRepos(p.out, cfg, paths, repos)
	if len(added) == 0 && len(dirs) == 0 {
		fmt.Fprintln(p.out, "No source repos yet. Add them later with gbridge add repo <path> or gbridge add scan <folder>.")
		return nil, nil
	}
	q := fmt.Sprintf("Use these %d repos?", len(added)+len(found))
	if len(dirs) > 0 {
		q = fmt.Sprintf("Use these %d repos, and search %s for new ones on every sync?", len(added)+len(found), strings.Join(dirs, ", "))
	}
	if !p.confirm(q) {
		return nil, errAborted
	}
	cfg.Repos = append(cfg.Repos, added...)
	cfg.ScanFolders = append(cfg.ScanFolders, dirs...)
	return found, nil
}

// pickRepos judges the repos the user named (paths) and the repos found in
// scan folders against cfg, says why on out, and sorts them: named repos to
// add to cfg.Repos, found repos worth mirroring, and named repos refused, all
// as absolute paths. Repos already in cfg.Repos are left alone. It reads each
// repo the way plan does.
func pickRepos(out io.Writer, cfg *config.Config, paths []string, found []sourceRepo) (added, kept, refused []string) {
	have := map[string]bool{}
	for _, r := range cfg.Repos {
		have[filepath.Clean(r)] = true
	}
	var repos []sourceRepo
	listed := map[string]bool{}
	for _, r := range append(namedRepos(paths), found...) {
		if !listed[r.path] {
			listed[r.path] = true
			repos = append(repos, r)
		}
	}
	ins := inspectAll(cfg, repos, time.Time{}, false)
	for i, r := range repos {
		named := r.scanFolder == ""
		switch {
		case have[r.path]:
			fmt.Fprintf(out, "  have  %s (already in config)\n", r.path)
		case !keepRepo(out, r.path, named, ins[i]):
			if named {
				refused = append(refused, r.path)
			}
		case named:
			added = append(added, r.path)
		default:
			kept = append(kept, r.path)
		}
	}
	return added, kept, refused
}

func namedRepos(paths []string) []sourceRepo {
	out := make([]sourceRepo, 0, len(paths))
	for _, p := range paths {
		out = append(out, sourceRepo{path: absPath(p)})
	}
	return out
}

// keepRepo reports whether repo r is worth mirroring, given what inspecting
// it found, and says why on out.
func keepRepo(out io.Writer, r string, named bool, in inspection) bool {
	switch {
	case in.err != nil:
		fmt.Fprintf(out, "  skip  %s (%v)\n", r, in.err)
		return false
	case in.hosted:
		fmt.Fprintf(out, "  skip  %s (github.com remote, GitHub already counts it; --include-github to override)\n", r)
		return false
	case len(in.acts) == 0 && !named:
		fmt.Fprintf(out, "  skip  %s (no commits by these emails)\n", r)
		return false
	case named:
		fmt.Fprintf(out, "  add   %s (%d commits)\n", r, len(in.acts))
	default:
		fmt.Fprintf(out, "  found %s (%d commits)\n", r, len(in.acts))
	}
	return true
}
