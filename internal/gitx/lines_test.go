package gitx_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/gitx"
)

func TestLinesYieldsEachLineInOrder(t *testing.T) {
	gittest.Need(t)
	repo := t.TempDir()
	gittest.Init(t, repo)
	for _, e := range []string{"a@x.example", "b@x.example", "c@x.example"} {
		gittest.Commit(t, repo, e, "2025-03-01T10:00:00Z")
	}
	var got []string
	for line, err := range gitx.Lines(repo, "log", "--format=%ae") {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, line)
	}
	if want := []string{"c@x.example", "b@x.example", "a@x.example"}; !slices.Equal(got, want) {
		t.Errorf("Lines() = %q, want %q", got, want)
	}
}

// A failing git ends the sequence with one error carrying git's stderr.
func TestLinesReportsGitsError(t *testing.T) {
	gittest.Need(t)
	var lines, errs []string
	for line, err := range gitx.Lines(t.TempDir(), "log") {
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) != 0 || len(errs) != 1 || !strings.Contains(errs[0], "not a git repository") {
		t.Errorf("Lines() in a plain folder = lines %q, errors %q; want one error quoting git", lines, errs)
	}
}

// Breaking out early stops git even when its output would fill the pipe,
// and leaves no goroutine behind.
func TestLinesStopsGitOnEarlyBreak(t *testing.T) {
	gittest.Need(t)
	repo := t.TempDir()
	gittest.Init(t, repo)
	msg := filepath.Join(t.TempDir(), "msg")
	if err := os.WriteFile(msg, []byte(strings.Repeat("a line of a very long message\n", 100_000)), 0o600); err != nil {
		t.Fatal(err)
	}
	identity := []string{ // CI runners have no global user.name or user.email
		"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL=dev@example.com",
	}
	gittest.Git(t, repo, identity, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-F", msg)

	before := runtime.NumGoroutine()
	done := make(chan []string)
	go func() {
		var got []string
		for line, err := range gitx.Lines(repo, "log", "--format=%B") {
			if err != nil {
				t.Error(err)
			}
			got = append(got, line)
			break
		}
		done <- got
	}()
	select {
	case got := <-done:
		if want := []string{"a line of a very long message"}; !slices.Equal(got, want) {
			t.Errorf("first line = %q, want %q", got, want)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Lines() still running 30s after the loop broke: git was never stopped")
	}
	for deadline := time.Now().Add(5 * time.Second); runtime.NumGoroutine() > before; {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines after the loop broke, %d before", runtime.NumGoroutine(), before)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
