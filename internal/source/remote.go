package source

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/sorfeb/greenbridge/internal/gitx"
)

// Remote compares one remote with the source repo's copy of it.
type Remote struct {
	Name string
	// Unfetched counts refs on the remote, under the repo's fetch refspecs,
	// whose tip commit the repo lacks. Sync cannot see those commits.
	Unfetched int
	// Err is set when the remote could not be listed: a gone path, a bad
	// URL, missing credentials, or the context ending.
	Err error
}

// CheckRemotes compares origin and every remote a local branch tracks with
// what repo holds. It lists each remote's refs with git ls-remote, which
// moves no objects, and never prompts for credentials. Leftover remotes no
// branch tracks are skipped. Only listing the remotes uses ctx; local git
// calls always run.
func CheckRemotes(ctx context.Context, repo string) ([]Remote, error) {
	names, err := gitx.Git(repo, "remote")
	if err != nil {
		return nil, err
	}
	// Exit status 1 means no key matched.
	conf, _ := gitx.Git(repo, "config", "--get-regexp", `^(branch\..*\.remote|remote\..*\.fetch|core\.sshcommand)$`)
	tracked, specs, sshConf := map[string]bool{"origin": true}, map[string][]string{}, false
	for line := range strings.SplitSeq(conf, "\n") {
		key, val, _ := strings.Cut(strings.TrimRight(line, "\r"), " ")
		switch {
		case key == "core.sshcommand":
			sshConf = true
		case strings.HasPrefix(key, "branch.") && strings.HasSuffix(key, ".remote"):
			tracked[val] = true
		case strings.HasPrefix(key, "remote.") && strings.HasSuffix(key, ".fetch"):
			r := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".fetch")
			specs[r] = append(specs[r], val)
		}
	}
	var rs []Remote
	for name := range strings.FieldsSeq(names) {
		if tracked[name] {
			rs = append(rs, Remote{Name: name})
		}
	}
	slices.SortStableFunc(rs, func(a, b Remote) int {
		if (a.Name == "origin") != (b.Name == "origin") {
			if a.Name == "origin" {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	if len(rs) == 0 {
		return nil, nil
	}

	local, err := localRefs(repo)
	if err != nil {
		return nil, err
	}
	env := noPromptEnv(os.Getenv, sshConf)
	for i := range rs {
		r := &rs[i]
		out, err := gitx.RunContext(ctx, repo, env, nil, "ls-remote", "--refs", r.Name)
		if err != nil {
			if ctx.Err() != nil {
				err = fmt.Errorf("listing %s: %w", r.Name, ctx.Err())
			}
			r.Err = err
			continue
		}
		r.Unfetched, r.Err = unfetched(repo, out, parseRefspecs(specs[r.Name]), local)
	}
	return rs, nil
}

// noPromptEnv keeps git from waiting on a person: no terminal prompt, no
// credential manager window, and ssh in batch mode unless the user already
// chose an ssh command, which a GIT_SSH_COMMAND set here would override.
func noPromptEnv(getenv func(string) string, sshConfigured bool) []string {
	env := []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}
	if getenv("GIT_SSH_COMMAND") == "" && getenv("GIT_SSH") == "" && !sshConfigured {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

// localRefs maps every ref in repo to the commit it names.
func localRefs(repo string) (map[string]string, error) {
	out, err := gitx.Git(repo, "for-each-ref", "--format=%(objectname) %(refname)")
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		if sha, ref, ok := strings.Cut(strings.TrimRight(line, "\r"), " "); ok {
			refs[ref] = sha
		}
	}
	return refs, nil
}

// unfetched counts the refs in ls-remote output that a fetch would update
// and whose commit repo lacks. A ref that moved to a commit the repo
// already holds hides nothing from sync.
func unfetched(repo, lsRemote string, specs []refspec, local map[string]string) (int, error) {
	var want []string
	for line := range strings.SplitSeq(lsRemote, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if !ok {
			continue
		}
		if dst, ok := mapRef(specs, ref); ok && local[dst] != sha {
			want = append(want, sha)
		}
	}
	if len(want) == 0 {
		return 0, nil
	}
	out, err := gitx.Run(repo, nil, strings.NewReader(strings.Join(want, "\n")+"\n"), "cat-file", "--batch-check")
	if err != nil {
		return 0, err
	}
	n := 0
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasSuffix(strings.TrimRight(line, "\r"), " missing") {
			n++
		}
	}
	return n, nil
}

// refspec is one remote.<name>.fetch value. src and dst hold at most one
// "*" each. A negative refspec excludes the refs src matches.
type refspec struct {
	src, dst string
	negative bool
}

func parseRefspecs(values []string) []refspec {
	specs := make([]refspec, 0, len(values))
	for _, v := range values {
		if neg, ok := strings.CutPrefix(v, "^"); ok {
			specs = append(specs, refspec{src: neg, negative: true})
			continue
		}
		src, dst, _ := strings.Cut(strings.TrimPrefix(v, "+"), ":")
		specs = append(specs, refspec{src: src, dst: dst})
	}
	return specs
}

// mapRef returns the local ref a fetch writes for remote ref, and false
// when no refspec stores it.
func mapRef(specs []refspec, ref string) (string, bool) {
	for _, s := range specs {
		if _, ok := matchGlob(s.src, ref); s.negative && ok {
			return "", false
		}
	}
	for _, s := range specs {
		if s.negative || s.dst == "" {
			continue
		}
		if star, ok := matchGlob(s.src, ref); ok {
			return strings.Replace(s.dst, "*", star, 1), true
		}
	}
	return "", false
}

// matchGlob matches ref against a pattern with at most one "*" and returns
// what the star stood for.
func matchGlob(pattern, ref string) (string, bool) {
	before, after, glob := strings.Cut(pattern, "*")
	if !glob {
		return "", pattern == ref
	}
	if len(ref) < len(before)+len(after) || !strings.HasPrefix(ref, before) || !strings.HasSuffix(ref, after) {
		return "", false
	}
	return ref[len(before) : len(ref)-len(after)], true
}
