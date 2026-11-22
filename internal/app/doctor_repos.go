package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

func checkRepos(f *facts) []result {
	if f.cfgErr != nil {
		return []result{skipped("repos", "config")}
	}
	if len(f.repos) == 0 && len(f.scans) == 0 {
		return []result{warn("repos", "no source repos or scan folders in config, nothing to mirror")}
	}
	var rs []result
	for _, r := range f.repos {
		switch {
		case r.missing:
			rs = append(rs, fail("repos", "source repo missing: "+r.path, "gbridge remove repo "+r.path+", or restore the clone"))
		case r.notGit:
			rs = append(rs, fail("repos", "not a git repo: "+r.path, "gbridge remove repo "+r.path))
		case r.hasGitHub && f.cfg.IncludeGitHubRepos:
			rs = append(rs, warn("repos", "github.com remote on an included repo, counts twice: "+r.path))
		default:
			rs = append(rs, ok("repos", "source repo "+r.path))
		}
	}
	for _, s := range f.scans {
		if s.missing {
			rs = append(rs, fail("repos", "scan folder missing: "+s.path, "gbridge remove scan "+s.path+", or restore the folder"))
		} else {
			rs = append(rs, ok("repos", fmt.Sprintf("scan folder %s (%d git repos)", s.path, s.repos)))
		}
	}
	return rs
}

// checkEmails warns about commits under an email missing from config. In a
// shared repo most unlisted authors are coworkers, so only commits under the
// user's own author name count: a name on commits by a configured email, or
// git's user.name. An email that also carries other names is reported but
// never suggested. Only the email and counts are printed.
func checkEmails(f *facts) []result {
	const check = "emails"
	if f.cfgErr != nil {
		return []result{skipped(check, "config")}
	}
	listed := map[string]bool{}
	for _, e := range f.cfg.Emails {
		listed[strings.ToLower(strings.TrimSpace(e))] = true
	}
	names := map[string]bool{}
	for _, r := range f.repos {
		if n := strings.ToLower(strings.TrimSpace(r.userName)); n != "" {
			names[n] = true
		}
		for _, a := range r.authors {
			if listed[a.Email] && a.Name != "" {
				names[strings.ToLower(a.Name)] = true
			}
		}
	}
	// others counts commits under other names on the same unlisted email: a
	// coworker's address the user once committed with. Adding it would mirror
	// the coworker's work as the user's, so such an email gets no fix.
	counts, others := map[string]int{}, map[string]int{}
	for _, r := range f.repos {
		for _, a := range r.authors {
			switch {
			case listed[a.Email]:
			case names[strings.ToLower(a.Name)]:
				counts[a.Email] += a.Commits
			default:
				others[a.Email] += a.Commits
			}
		}
	}
	if len(counts) == 0 {
		return []result{ok(check, "no commits in "+emailWindow+" under your author name and an unlisted email")}
	}
	emails := slices.Collect(maps.Keys(counts))
	slices.SortFunc(emails, func(a, b string) int { return cmp.Or(counts[b]-counts[a], strings.Compare(a, b)) })
	rs := make([]result, 0, len(emails))
	for _, e := range emails {
		mine := fmt.Sprintf("%s in %s under your author name use %s", plural(counts[e], "commit"), emailWindow, e)
		if others[e] > 0 {
			rs = append(rs, ok(check, fmt.Sprintf("%s, which also has %s under other author names: left out",
				mine, plural(others[e], "commit"))))
			continue
		}
		rs = append(rs, result{check, statusWarn, mine + ", which is not in config emails", "gbridge add email " + e})
	}
	return rs
}

// checkRemotes warns about tracked remotes holding commits the repo lacks,
// and about remotes git can't reach. A remote with nothing new is fine
// however long ago the repo fetched it. Remotes too slow to answer share
// one line: a slow forge is not a broken remote. It is an online check.
func checkRemotes(f *facts) []result {
	const check = "remotes"
	switch {
	case f.cfgErr != nil:
		return []result{skipped(check, "config")}
	case !f.online:
		return []result{skipped(check, "gbridge doctor --online")}
	}
	var rs []result
	current, slow := 0, 0
	for _, r := range f.repos {
		p := shellPath(r.path)
		for _, rem := range r.remotes {
			switch {
			case errors.Is(rem.Err, context.DeadlineExceeded):
				slow++
			case rem.Err != nil:
				rs = append(rs, result{
					check, statusWarn, "can't reach " + rem.Name + ": " + r.path,
					"git -C " + p + " remote remove " + rem.Name + ", or fix its URL: git -C " + p + " remote set-url " + rem.Name + " <url>",
				})
			case rem.Unfetched > 0:
				what := fmt.Sprintf("%d branches on %s have", rem.Unfetched, rem.Name)
				if rem.Unfetched == 1 {
					what = "1 branch on " + rem.Name + " has"
				}
				rs = append(rs, result{
					check, statusWarn, what + " commits you haven't fetched: " + r.path,
					"git -C " + p + " fetch " + rem.Name,
				})
			default:
				current++
			}
		}
	}
	switch {
	case current > 0:
		rs = append(rs, ok(check, plural(current, "tracked remote")+" checked, nothing left to fetch"))
	case len(rs) == 0 && slow == 0:
		rs = append(rs, ok(check, "no source repo tracks a remote"))
	}
	if slow > 0 {
		rs = append(rs, unverified(check,
			fmt.Sprintf("%s gave no answer within %s, not checked", plural(slow, "tracked remote"), remoteTimeout),
			"run gbridge doctor --online again, or git fetch in those repos"))
	}
	return rs
}

// shellPath quotes p for a fix line when it holds a space.
func shellPath(p string) string {
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
