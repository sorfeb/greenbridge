package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/gittest"
	"github.com/sorfeb/greenbridge/internal/hook"
	"github.com/sorfeb/greenbridge/internal/ident"
)

// pushSetup is a built gbridge in a folder with a space, a greenbridge home
// mirroring one source repo to a local bare repo, and a global git config
// of its own, so nothing touches the machine's.
type pushSetup struct {
	bin, home, src, remote string
}

func newPushSetup(t *testing.T) pushSetup {
	t.Helper()
	gittest.Need(t)
	tmp := t.TempDir()
	gcfg := filepath.Join(tmp, "gitconfig")
	if err := os.WriteFile(gcfg, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gcfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	s := pushSetup{
		bin:    filepath.Join(tmp, "Program Files", "gbridge"),
		home:   filepath.Join(tmp, "home"),
		src:    filepath.Join(tmp, "src"),
		remote: filepath.Join(tmp, "mirror-remote.git"),
	}
	if runtime.GOOS == "windows" {
		s.bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", s.bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	t.Setenv("GBRIDGE_HOME", s.home)

	gittest.Init(t, s.src)
	gittest.InitBare(t, s.src+"-origin.git")
	gittest.Git(t, s.src, nil, "remote", "add", "origin", s.src+"-origin.git")
	gittest.InitBare(t, s.remote)
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Emails = []string{"me@work.example"}
	cfg.Repos = []string{s.src}
	cfg.Mirror.Remote = s.remote
	cfg.MirrorAuthor = config.MirrorAuthor{Name: "me", Email: "1+me@users.noreply.github.com"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	sp, _ := config.SecretPath()
	if err := ident.Save(sp, bytes.Repeat([]byte{7}, ident.SecretSize)); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s pushSetup) gbridge(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command(s.bin, args...).CombinedOutput()
	return string(out), err
}

// push commits and pushes the source repo, and reports how long the push
// took.
func (s pushSetup) push(t *testing.T, date string) (time.Duration, error) {
	t.Helper()
	gittest.Commit(t, s.src, "me@work.example", date)
	start := time.Now()
	out, err := exec.Command("git", "-C", s.src, "push", "-q", "origin", "HEAD:main").CombinedOutput()
	if err != nil {
		t.Logf("git push: %s", out)
	}
	return time.Since(start), err
}

// waitSynced waits for a background sync to log a result containing want,
// then for its process to exit, which on Windows frees the binary.
func (s pushSetup) waitSynced(t *testing.T, want string) string {
	t.Helper()
	f := hook.FilesIn(s.home)
	var log []byte
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		log, _ = os.ReadFile(f.Log)
		if !bytes.Contains(log, []byte(want)) {
			continue
		}
		if release, ok, err := hook.Hold(f); err == nil && ok {
			release()
			return string(log)
		}
	}
	t.Fatalf("no background sync logged %q; sync.log:\n%s", want, log)
	return ""
}

func (s pushSetup) mirrored(t *testing.T) string {
	t.Helper()
	return gittest.Git(t, s.remote, nil, "rev-list", "--count", "main")
}

// A push in a source repo starts a background sync that mirrors it, and a
// push hook whose binary is gone never fails the push.
func TestPushStartsBackgroundSync(t *testing.T) {
	for _, perRepo := range []bool{false, true} {
		name := "global"
		if perRepo {
			name = "per-repo"
		}
		t.Run(name, func(t *testing.T) {
			s := newPushSetup(t)
			args := []string{"hook", "install"}
			if perRepo {
				args = append(args, "--per-repo")
			}
			out, err := s.gbridge(t, args...)
			if !perRepo && !gitVersion(t).ConfigHooks() {
				if err == nil || !strings.Contains(out, "--per-repo") {
					t.Errorf("install on Git before 2.54: %v\n%s\nwant a refusal naming --per-repo", err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("gbridge %s: %v\n%s", strings.Join(args, " "), err, out)
			}

			took, err := s.push(t, "2025-01-02T10:00:00Z")
			if err != nil {
				t.Fatalf("push with the hook installed: %v", err)
			}
			t.Logf("push took %v", took)
			log := s.waitSynced(t, "background sync ok: 1 desired, 0 already mirrored, added 1")
			if n := s.mirrored(t); n != "1" {
				t.Errorf("mirror remote holds %s commits, want 1", n)
			}
			if strings.Contains(log, "\x1b[") {
				t.Errorf("sync.log holds terminal escapes:\n%q", log)
			}

			if err := os.Remove(s.bin); err != nil {
				t.Fatal(err)
			}
			if _, err := s.push(t, "2025-01-03T10:00:00Z"); err != nil {
				t.Errorf("push with the binary gone failed: %v", err)
			}
		})
	}
}

func gitVersion(t *testing.T) hook.Version {
	t.Helper()
	v, err := hook.GitVersion()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Opting a repo out stops the global push hook there.
func TestPushHookOptOut(t *testing.T) {
	s := newPushSetup(t)
	if !gitVersion(t).ConfigHooks() {
		t.Skip("config hooks need Git 2.54 or later")
	}
	if out, err := s.gbridge(t, "hook", "install"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	gittest.Git(t, s.src, nil, "config", "hook.gbridge.enabled", "false")
	if _, err := s.push(t, "2025-01-02T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	if _, err := os.Stat(hook.FilesIn(s.home).Log); err == nil {
		log, _ := os.ReadFile(hook.FilesIn(s.home).Log)
		t.Errorf("opted-out repo started a background sync:\n%s", log)
	}
}
