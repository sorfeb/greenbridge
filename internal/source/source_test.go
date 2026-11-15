package source

import (
	"slices"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/gittest"
)

// Authors walks the same refs as Scan. Editor checkpoint refs such as
// refs/t3/* hold thousands of commits under the tool's own identity; counting
// them would flag the tool's email as one of the user's.
func TestAuthorsReadsScanRefsOnly(t *testing.T) {
	gittest.Need(t)
	repo := t.TempDir()
	gittest.Init(t, repo)
	gittest.CommitAs(t, repo, "Me Dev", "me@work.example", "2025-03-01T10:00:00Z")
	gittest.CommitAs(t, repo, "Me Dev", "Me@Work.example", "2025-03-02T10:00:00Z")
	gittest.Git(t, repo, nil, "tag", "v1")
	gittest.CommitOnRef(t, repo, "refs/remotes/origin/feature", "Me Dev", "old@laptop.example", "2025-03-03T10:00:00Z")
	gittest.CommitOnRef(t, repo, "refs/t3/checkpoints/abc/turn/1", "Me Dev", "t3code@users.noreply.github.com", "2025-03-04T10:00:00Z")
	gittest.CommitOnRef(t, repo, "refs/notes/commits", "Me Dev", "notes@laptop.example", "2025-03-05T10:00:00Z")
	gittest.CommitAs(t, repo, "Me Dev", "ancient@laptop.example", "2020-01-01T10:00:00Z")

	got, err := Authors(repo, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := []Author{
		{Name: "Me Dev", Email: "me@work.example", Commits: 2},
		{Name: "Me Dev", Email: "old@laptop.example", Commits: 1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Authors() = %+v, want %+v", got, want)
	}

	// Scan agrees on which commits exist: the checkpoint is invisible to both.
	acts, err := Scan(repo, []string{"t3code@users.noreply.github.com", "old@laptop.example"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Email != "old@laptop.example" {
		t.Errorf("Scan() = %+v, want only the remote-tracking commit", acts)
	}
}
