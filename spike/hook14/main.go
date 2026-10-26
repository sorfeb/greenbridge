// Command gbspike is the issue #14 spike: a pre-push hook that records what
// Git hands it, then either starts a detached sync or appends to a queue.
// Throwaway reference code; nothing here ships.
//
//	gbspike hook <remote> <url>   pre-push entry point, always exits 0
//	gbspike sync                  stand-in for `gbridge sync`: lock, sleep, log
//	gbspike probe                 logs its console state; sync runs it the way gbridge runs git
//
// Environment:
//
//	GBSPIKE_LOG        JSON-lines log (default $TMPDIR/gbspike.log)
//	GBSPIKE_MODE       detach | inherit | queue | none (default detach)
//	GBSPIKE_SYNC_SECS  how long the fake sync runs (default 3)
//	GBSPIKE_EXIT       exit code for `hook` (default 0; only to prove why 0 matters)
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const zeroSHA = "0000000000000000000000000000000000000000"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gbspike hook|sync")
		return
	}
	switch os.Args[1] {
	case "hook":
		code := 0
		if v := os.Getenv("GBSPIKE_EXIT"); v != "" {
			code, _ = strconv.Atoi(v)
		}
		func() {
			// The hook never fails the push, not even on a panic.
			defer func() {
				if r := recover(); r != nil {
					logEvent(map[string]any{"ev": "hook-panic", "err": fmt.Sprint(r)})
				}
			}()
			hook(os.Args[2:])
		}()
		os.Exit(code)
	case "sync":
		syncMain()
	case "probe":
		logEvent(map[string]any{"ev": "probe", "console": consoleState()})
	}
}

func hook(args []string) {
	start := time.Now()
	var lines []string
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	mode := env("GBSPIKE_MODE", "detach")
	ev := map[string]any{
		"ev": "hook", "mode": mode, "args": args, "stdin": lines,
		"cwd": wd(), "git_env": gitEnv(), "ppid": os.Getppid(),
	}
	switch mode {
	case "detach":
		ev["spawn_err"] = errString(spawn(false))
	case "inherit":
		ev["spawn_err"] = errString(spawn(true))
	case "queue":
		n, err := enqueue(args, lines)
		ev["queued"], ev["queue_err"] = n, errString(err)
	}
	ev["hook_ms"] = time.Since(start).Milliseconds()
	logEvent(ev)
}

// spawn starts `gbspike sync`. inherit=true hands the child our stdout and
// stderr, which is the mistake runners like pre-commit punish: they read the
// pipe until EOF, and the child holds it open.
func spawn(inherit bool) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "sync")
	cmd.Env = scrubGitEnv(os.Environ())
	if inherit {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// scrubGitEnv drops GIT_* variables a hook inherits (GIT_DIR, GIT_INDEX_FILE,
// ...). Left in, the sync's own git calls would hit the pushing repo instead
// of the mirror.
func scrubGitEnv(in []string) []string {
	out := in[:0:0]
	for _, kv := range in {
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") {
			out = append(out, kv)
		}
	}
	return append(out, "GIT_TERMINAL_PROMPT=0")
}

// enqueue appends author timestamps of the pushed commits to a queue file.
// Each stdin line is "<local ref> <local sha> <remote ref> <remote sha>".
func enqueue(args, lines []string) (int, error) {
	remote := ""
	if len(args) > 0 {
		remote = args[0]
	}
	f, err := os.OpenFile(filepath.Join(filepath.Dir(logPath()), "gbspike.queue"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	n := 0
	for _, l := range lines {
		fl := strings.Fields(l)
		if len(fl) != 4 || !isSHA(fl[1]) || !isSHA(fl[3]) || fl[1] == zeroSHA { // malformed or a delete
			continue
		}
		rng := []string{fl[1], "--not", "--remotes=" + remote}
		if fl[3] != zeroSHA {
			rng = []string{fl[3] + ".." + fl[1]}
		}
		//nolint:gosec // G702: SHAs are validated hex, the remote name is one argv element, no shell
		out, err := exec.Command("git", append([]string{"log", "--format=%ae %at"}, rng...)...).Output()
		if err != nil {
			return n, err
		}
		for s := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
			if s != "" {
				fmt.Fprintln(f, s)
				n++
			}
		}
	}
	return n, nil
}

func isSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	return strings.Trim(s, "0123456789abcdef") == ""
}

func syncMain() {
	lock := filepath.Join(filepath.Dir(logPath()), "gbspike.lock")
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		logEvent(map[string]any{"ev": "sync-skip-locked"})
		return
	}
	if err != nil {
		logEvent(map[string]any{"ev": "sync-err", "err": err.Error()})
		return
	}
	defer func() { _ = f.Close(); _ = os.Remove(lock) }()
	logEvent(map[string]any{"ev": "sync-start", "ppid": os.Getppid(), "git_env": gitEnv(), "console": consoleState()})
	if self, err := os.Executable(); err == nil {
		_ = exec.Command(self, "probe").Run()
	}
	secs, _ := strconv.Atoi(env("GBSPIKE_SYNC_SECS", "3"))
	// Writing to stdout here is what an inheriting child would do: progress output.
	fmt.Println("gbspike sync: working")
	time.Sleep(time.Duration(secs) * time.Second)
	logEvent(map[string]any{"ev": "sync-done"})
}

func logEvent(ev map[string]any) {
	ev["t"] = time.Now().Format("15:04:05.000")
	ev["pid"] = os.Getpid()
	b, _ := json.Marshal(ev)
	f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

func logPath() string { return env("GBSPIKE_LOG", filepath.Join(os.TempDir(), "gbspike.log")) }

func gitEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "GIT_") && !strings.HasPrefix(kv, "GIT_CONFIG_GLOBAL") {
			out = append(out, kv)
		}
	}
	return out
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func wd() string { d, _ := os.Getwd(); return d }

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
