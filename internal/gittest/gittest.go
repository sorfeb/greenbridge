// Package gittest builds throwaway git repos for tests in any package.
package gittest

import (
	"os/exec"
	"testing"

	"github.com/sorfeb/greenbridge/internal/gitx"
)

// Message is the subject of every commit made by Commit. A mirror that
// contains it has leaked source data.
const Message = "secret work message"

// Need skips the test when git is not installed.
func Need(tb testing.TB) {
	tb.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		tb.Skip("git not installed")
	}
}

// Git runs git in dir with extra env entries and returns its output. Any
// error fails the test.
func Git(tb testing.TB, dir string, env []string, args ...string) string {
	tb.Helper()
	out, err := gitx.Run(dir, env, nil, args...)
	if err != nil {
		tb.Fatal(err)
	}
	return out
}

// Init creates a repo at dir with main as its initial branch.
func Init(tb testing.TB, dir string) {
	tb.Helper()
	Git(tb, "", nil, "init", "-q", "-b", "main", dir)
}

// InitBare creates a bare repo at dir with main as its initial branch.
func InitBare(tb testing.TB, dir string) {
	tb.Helper()
	Git(tb, "", nil, "init", "-q", "--bare", "-b", "main", dir)
}

// Commit adds an empty commit to repo, authored and committed by email at
// date (RFC 3339).
func Commit(tb testing.TB, repo, email, date string) {
	tb.Helper()
	CommitAs(tb, repo, "w", email, date)
}

// CommitAs is Commit with an author and committer name.
func CommitAs(tb testing.TB, repo, name, email, date string) {
	tb.Helper()
	env := []string{
		"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email, "GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email, "GIT_COMMITTER_DATE=" + date,
	}
	Git(tb, repo, env, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", Message)
}

// CommitOnRef adds a commit reachable only from ref, the way tools such as
// editor checkpoints write under refs/<tool>/. HEAD must already have a
// commit; the checked-out branch stays where it was.
func CommitOnRef(tb testing.TB, repo, ref, name, email, date string) {
	tb.Helper()
	branch := Git(tb, repo, nil, "symbolic-ref", "--short", "HEAD")
	Git(tb, repo, nil, "checkout", "-q", "--detach")
	CommitAs(tb, repo, name, email, date)
	Git(tb, repo, nil, "update-ref", ref, "HEAD")
	Git(tb, repo, nil, "checkout", "-q", branch)
}
