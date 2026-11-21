package hook

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BackgroundEnv is set to 1 in a background sync's environment. sync reads
// it to run under the lock and log its result, and the push hook reads it to
// stay quiet when the background sync's own push to the mirror repo fires it.
const BackgroundEnv = "GBRIDGE_BACKGROUND"

// maxRounds bounds how often one background sync runs again for triggers
// that arrived while it ran.
const maxRounds = 3

// maxLog is the size past which the log moves to sync.log.old before a new
// background sync starts.
const maxLog = 1 << 20

// Files is where background syncs keep their state in the greenbridge home.
type Files struct {
	Log   string // output of every background sync, plus one result line each
	Lock  string // held while a background sync runs
	Dirty string // present when a trigger arrived while the lock was held
}

// FilesIn returns the files in greenbridge home dir.
func FilesIn(home string) Files {
	return Files{
		Log:   filepath.Join(home, "sync.log"),
		Lock:  filepath.Join(home, "sync.lock"),
		Dirty: filepath.Join(home, "sync.dirty"),
	}
}

// Detach starts exe with args in the background and returns once it has
// started. The child gets the log as stdout and stderr and the null device
// as stdin, so no hook runner waits on a pipe it holds; it runs in the
// greenbridge home, in its own session or process group, without a console
// window.
func Detach(exe string, args []string, f Files) error {
	home := filepath.Dir(f.Log)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	if fi, err := os.Stat(f.Log); err == nil && fi.Size() > maxLog {
		_ = os.Rename(f.Log, f.Log+".old") // best effort: a lost rotation only grows the log
	}
	log, err := os.OpenFile(f.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	cmd := exec.Command(exe, args...)
	cmd.Dir = home
	cmd.Env = childEnv(os.Environ())
	cmd.Stdout, cmd.Stderr = log, log
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// keptGitEnv are the GIT_* variables a background sync keeps: the user's
// config and transport choices. Every other GIT_* comes from the push that
// fired the hook (GIT_DIR, GIT_EXEC_PATH, GIT_PREFIX, ...) and would point
// the sync's git calls at the pushing repo.
var keptGitEnv = map[string]bool{
	"GIT_CONFIG_GLOBAL": true, "GIT_CONFIG_SYSTEM": true, "GIT_CONFIG_NOSYSTEM": true,
	"GIT_SSH": true, "GIT_SSH_COMMAND": true, "GIT_SSH_VARIANT": true, "GIT_ASKPASS": true,
	"GIT_SSL_CAINFO": true, "GIT_SSL_CAPATH": true, "GIT_PROXY_COMMAND": true,
}

// childEnv is environ for a background sync: the push's GIT_* variables
// dropped, no terminal prompts and no credential manager windows (nobody is
// there to answer them; a failure goes to the log), BackgroundEnv set.
func childEnv(environ []string) []string {
	out := make([]string, 0, len(environ)+3)
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if k == BackgroundEnv || k == "GCM_INTERACTIVE" || strings.HasPrefix(strings.ToUpper(k), "GIT_") && !keptGitEnv[k] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", BackgroundEnv+"=1")
}

// Serialize runs sync under the lock and writes a result line for each run
// to log. When another background sync holds the lock, it marks that one
// dirty and returns: the running sync goes once more when it finishes, and
// the next push catches anything after that.
func Serialize(f Files, log io.Writer, sync func() (summary string, err error)) {
	release, ok, err := tryLock(f.Lock)
	if err != nil {
		logLine(log, "failed", err.Error())
		return
	}
	if !ok {
		if err := os.WriteFile(f.Dirty, nil, 0o600); err != nil {
			logLine(log, "failed", err.Error())
			return
		}
		logLine(log, "skipped", "another background sync is running and goes once more when it finishes")
		return
	}
	defer release()
	for range maxRounds {
		_ = os.Remove(f.Dirty)
		logLine(log, "started", fmt.Sprintf("pid %d", os.Getpid()))
		if summary, err := sync(); err != nil {
			logLine(log, "failed", err.Error())
		} else {
			logLine(log, "ok", summary)
		}
		if !exists(f.Dirty) {
			return
		}
	}
	_ = os.Remove(f.Dirty) // the next push catches up
}

const linePrefix = " background sync "

// logLine writes one result line: an RFC 3339 time, then the outcome.
func logLine(w io.Writer, outcome, msg string) {
	msg = strings.Join(strings.Fields(msg), " ")
	fmt.Fprintf(w, "%s%s%s: %s\n", time.Now().Format(time.RFC3339), linePrefix, outcome, msg)
}

// Result is the outcome of one background sync, read back from the log.
type Result struct {
	Time time.Time
	OK   bool
	Msg  string
}

// LastResult reads the last finished background sync from the log at path.
// ok is false when there is none yet.
func LastResult(path string) (r Result, ok bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	for line := range strings.Lines(string(b)) {
		ts, rest, found := strings.Cut(strings.TrimSpace(line), linePrefix)
		if !found {
			continue
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		outcome, msg, _ := strings.Cut(rest, ": ")
		if outcome == "ok" || outcome == "failed" {
			r, ok = Result{Time: t, OK: outcome == "ok", Msg: msg}, true
		}
	}
	return r, ok, nil
}

// Hold takes the lock a foreground sync shares with background syncs, so
// the two never write the mirror clone at once. ok is false while a
// background sync holds it.
func Hold(f Files) (release func(), ok bool, err error) {
	if err := os.MkdirAll(filepath.Dir(f.Lock), 0o700); err != nil {
		return nil, false, err
	}
	return tryLock(f.Lock)
}

// HoldWait is Hold, retrying every poll until timeout while a background
// sync holds the lock. ok is false when the lock is still held at timeout.
func HoldWait(f Files, timeout, poll time.Duration) (release func(), ok bool, err error) {
	deadline := time.Now().Add(timeout)
	for {
		release, ok, err = Hold(f)
		if err != nil || ok || !time.Now().Before(deadline) {
			return release, ok, err
		}
		time.Sleep(min(poll, time.Until(deadline)))
	}
}

// LogFailure appends a failed result line to the log, for errors the push
// hook can tell nobody else about.
func LogFailure(f Files, msg string) {
	if err := os.MkdirAll(filepath.Dir(f.Log), 0o700); err != nil {
		return
	}
	log, err := os.OpenFile(f.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = log.Close() }()
	logLine(log, "failed", msg)
}
