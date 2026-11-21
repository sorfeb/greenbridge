package source

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/gittest"
)

var since2025 = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// One history read serves both views: activity for the configured emails,
// and every author on the same refs. Editor checkpoint refs such as
// refs/t3/* hold thousands of commits under the tool's own identity; counting
// them would flag the tool's email as one of the user's.
func TestHistoryReadsBranchesRemotesAndTagsOnly(t *testing.T) {
	gittest.Need(t)
	repo := t.TempDir()
	gittest.Init(t, repo)
	gittest.CommitAs(t, repo, "Me Dev", "me@work.example", "2025-03-01T10:00:00Z")
	gittest.CommitAs(t, repo, " Me Dev ", "Me@Work.example", "2025-03-02T10:00:00+07:00")
	gittest.CommitAs(t, repo, "Pat Coworker", "pat@work.example", "2025-03-02T11:00:00Z")
	gittest.Git(t, repo, nil, "tag", "v1")
	gittest.CommitOnRef(t, repo, "refs/remotes/origin/feature", "Me Dev", "old@laptop.example", "2025-03-03T10:00:00Z")
	gittest.CommitOnRef(t, repo, "refs/t3/checkpoints/abc/turn/1", "Me Dev", "t3code@users.noreply.github.com", "2025-03-04T10:00:00Z")
	gittest.CommitOnRef(t, repo, "refs/notes/commits", "Me Dev", "notes@laptop.example", "2025-03-05T10:00:00Z")
	gittest.CommitAs(t, repo, "Me Dev", "me@work.example", "2020-01-01T10:00:00Z")

	in, err := Inspect(repo, InspectOptions{SkipGitHub: true})
	if err != nil {
		t.Fatal(err)
	}
	if in.GitHubRemote || in.History == nil {
		t.Fatalf("Inspect() = %+v, want a read history and no github.com remote", in)
	}

	authors := in.History.Authors(since2025)
	wantAuthors := []Author{
		{Name: "Me Dev", Email: "me@work.example", Commits: 2},
		{Name: "Me Dev", Email: "old@laptop.example", Commits: 1},
		{Name: "Pat Coworker", Email: "pat@work.example", Commits: 1},
	}
	if !slices.Equal(authors, wantAuthors) {
		t.Errorf("Authors() = %+v, want %+v", authors, wantAuthors)
	}

	acts := in.History.Activity([]string{"ME@work.example ", "old@laptop.example", "t3code@users.noreply.github.com"}, since2025)
	var got []string
	for _, a := range acts {
		if a.Repo != repo || len(a.SHA) != 40 {
			t.Errorf("activity %+v: want repo %s and a full sha", a, repo)
		}
		got = append(got, a.Email+" "+a.Time.Format(time.RFC3339))
	}
	slices.Sort(got)
	want := []string{
		"me@work.example 2025-03-01T10:00:00Z",
		"me@work.example 2025-03-02T10:00:00+07:00", // original offset kept
		"old@laptop.example 2025-03-03T10:00:00Z",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Activity() = %q, want %q", got, want)
	}

	if all := in.History.Activity([]string{"me@work.example"}, time.Time{}); len(all) != 3 {
		t.Errorf("Activity() with zero since = %d commits, want 3 (all history)", len(all))
	}
}

// A repo with a github.com remote is skipped before git log runs when the
// caller leaves those out, and read in full when it includes them.
func TestInspectGitHubRemote(t *testing.T) {
	gittest.Need(t)
	repo := t.TempDir()
	gittest.Init(t, repo)
	gittest.Commit(t, repo, "me@work.example", "2025-03-01T10:00:00Z")
	gittest.Git(t, repo, nil, "remote", "add", "origin", "https://GitHub.com/me/app.git")

	in, err := Inspect(repo, InspectOptions{SkipGitHub: true})
	if err != nil || !in.GitHubRemote || in.History != nil {
		t.Errorf("Inspect(skip) = %+v, %v; want a github.com remote and no history", in, err)
	}
	in, err = Inspect(repo, InspectOptions{})
	if err != nil || !in.GitHubRemote || in.History == nil || len(in.History.Activity([]string{"me@work.example"}, time.Time{})) != 1 {
		t.Errorf("Inspect(include) = %+v, %v; want a github.com remote and one commit", in, err)
	}
}

func TestInspectUnreadable(t *testing.T) {
	gittest.Need(t)
	for name, dir := range map[string]string{
		"plain folder": t.TempDir(),
		"missing":      filepath.Join(t.TempDir(), "gone"),
	} {
		if _, err := Inspect(dir, InspectOptions{SkipGitHub: true}); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s: Inspect() = %v, want ErrUnreadable", name, err)
		}
	}
}

// A repo with no commits yet is readable and holds nothing.
func TestInspectEmptyRepo(t *testing.T) {
	gittest.Need(t)
	repo := t.TempDir()
	gittest.Init(t, repo)
	in, err := Inspect(repo, InspectOptions{SkipGitHub: true})
	if err != nil || in.History == nil {
		t.Fatalf("Inspect() = %+v, %v; want an empty history", in, err)
	}
	if acts, authors := in.History.Activity([]string{"me@work.example"}, time.Time{}), in.History.Authors(time.Time{}); len(acts)+len(authors) != 0 {
		t.Errorf("empty repo: activity %v authors %v, want none", acts, authors)
	}
}
