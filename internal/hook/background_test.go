package hook

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMain doubles as the background child Detach starts in TestDetach: a
// process that prints, sleeps, prints again, and records its environment.
func TestMain(m *testing.M) {
	if os.Getenv("GBRIDGE_HOOK_TEST_CHILD") == "1" {
		fmt.Println("child started")
		time.Sleep(1500 * time.Millisecond)
		fmt.Println("child env:", strings.Join(os.Environ(), "|"))
		fmt.Println("child done")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Detach returns while the child still runs, and the child's output lands
// in the log, never in the caller's stdout.
func TestDetach(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := FilesIn(filepath.Join(t.TempDir(), "home dir"))
	t.Setenv("GBRIDGE_HOOK_TEST_CHILD", "1")
	t.Setenv("GIT_DIR", "/pushing/repo/.git")
	t.Setenv("GIT_CONFIG_GLOBAL", "/kept/gitconfig")

	start := time.Now()
	if err := Detach(exe, []string{"-test.run=^$"}, f); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Detach took %v, want it back before the child finishes", d)
	}
	var log string
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		b, _ := os.ReadFile(f.Log)
		if log = string(b); strings.Contains(log, "child done") {
			break
		}
	}
	if !strings.Contains(log, "child started") || !strings.Contains(log, "child done") {
		t.Fatalf("log = %q, want the child's whole output", log)
	}
	_, env, _ := strings.Cut(log, "child env:")
	for _, want := range []string{BackgroundEnv + "=1", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/kept/gitconfig"} {
		if !strings.Contains(env, want) {
			t.Errorf("child env lacks %s", want)
		}
	}
	if strings.Contains(env, "GIT_DIR=") {
		t.Error("child inherited GIT_DIR: its git calls would hit the pushing repo")
	}
}

func TestChildEnv(t *testing.T) {
	in := []string{
		"PATH=/bin", "GIT_DIR=.git", "GIT_EXEC_PATH=/x", "GIT_PREFIX=sub/", "GIT_INDEX_FILE=i",
		"git_work_tree=/w", "GIT_CONFIG_GLOBAL=/g", "GIT_SSH_COMMAND=ssh -i k", "GIT_TERMINAL_PROMPT=1", "HOME=/h",
		"GCM_INTERACTIVE=always", // a credential window would pop up from a background push
	}
	got := childEnv(in)
	slices.Sort(got)
	want := []string{
		BackgroundEnv + "=1", "GCM_INTERACTIVE=never", "GIT_CONFIG_GLOBAL=/g", "GIT_SSH_COMMAND=ssh -i k", "GIT_TERMINAL_PROMPT=0",
		"HOME=/h", "PATH=/bin",
	}
	if !slices.Equal(got, want) {
		t.Errorf("childEnv =\n%q\nwant\n%q", got, want)
	}
}

func TestSerializeSkipsWhileLocked(t *testing.T) {
	f := FilesIn(t.TempDir())
	var outer, inner bytes.Buffer
	runs := 0
	Serialize(f, &outer, func() (string, error) {
		runs++
		if runs == 1 {
			// A second trigger while this run holds the lock.
			Serialize(f, &inner, func() (string, error) {
				t.Error("second sync ran while the first held the lock")
				return "", nil
			})
		}
		return fmt.Sprintf("round %d", runs), nil
	})
	if !strings.Contains(inner.String(), "background sync skipped") {
		t.Errorf("second trigger logged %q, want a skip", inner.String())
	}
	if runs != 2 {
		t.Errorf("first sync ran %d times, want 2: the skipped trigger asks for one more round", runs)
	}
	if _, err := os.Stat(f.Dirty); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dirty flag left behind: %v", err)
	}
	// The lock is free again.
	again := false
	Serialize(f, &outer, func() (string, error) { again = true; return "", nil })
	if !again {
		t.Error("lock still held after the sync finished")
	}
}

// A trigger that keeps arriving can't keep a sync alive forever.
func TestSerializeBoundsRounds(t *testing.T) {
	f := FilesIn(t.TempDir())
	runs := 0
	Serialize(f, &bytes.Buffer{}, func() (string, error) {
		runs++
		return "", os.WriteFile(f.Dirty, nil, 0o600)
	})
	if runs != maxRounds {
		t.Errorf("ran %d rounds, want %d", runs, maxRounds)
	}
}

func TestLastResult(t *testing.T) {
	f := FilesIn(t.TempDir())
	if _, ok, err := LastResult(f.Log); ok || err != nil {
		t.Errorf("no log: ok=%v err=%v, want neither", ok, err)
	}
	var log bytes.Buffer
	Serialize(f, &log, func() (string, error) { return "3 desired, 1 already mirrored, added 2", nil })
	Serialize(f, &log, func() (string, error) { return "", errors.New("push failed:\nremote said no") })
	log.WriteString("noise from git that looks like nothing\n")
	if err := os.WriteFile(f.Log, log.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	r, ok, err := LastResult(f.Log)
	if err != nil || !ok {
		t.Fatalf("LastResult: ok=%v err=%v", ok, err)
	}
	if r.OK || r.Msg != "push failed: remote said no" || time.Since(r.Time) > time.Minute {
		t.Errorf("LastResult = %+v, want the failure, on one line, just now", r)
	}
}
