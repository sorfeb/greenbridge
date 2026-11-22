// Package hook installs the push hook that starts a background sync on every
// git push, and runs that background sync.
//
// On Git 2.54 and later the push hook lives in global git config
// (hook.gbridge.*), so no repo's files change and teammates never see it.
// Older Git ignores that config without a word, so the fallback writes a
// pre-push script into each source repo's .git/hooks, which git never
// clones or pushes.
//
// The push hook only starts a background sync. It never reads stdin, never
// pushes inline, and always exits 0: a hook that fails, fails the push.
package hook

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sorfeb/greenbridge/internal/gitx"
)

// Name is the push hook's name in git config: hook.gbridge.*.
const Name = "gbridge"

// Event is the git hook event the push hook runs on.
const Event = "pre-push"

// Version is a git version.
type Version struct{ Major, Minor, Patch int }

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// ConfigHooks reports whether this git runs hooks defined in config.
// Earlier versions ignore hook.* silently.
func (v Version) ConfigHooks() bool {
	return v.Major > 2 || v.Major == 2 && v.Minor >= 54
}

var versionPattern = regexp.MustCompile(`^git version (\d+)\.(\d+)(?:\.(\d+))?`)

// ParseVersion reads the output of git --version.
func ParseVersion(s string) (Version, error) {
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Version{}, fmt.Errorf("unrecognized git version %q", strings.TrimSpace(s))
	}
	var v Version
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		v.Patch, _ = strconv.Atoi(m[3])
	}
	return v, nil
}

// GitVersion is the version of the git on PATH, the one gbridge runs.
func GitVersion() (Version, error) {
	out, err := gitx.Git("", "--version")
	if err != nil {
		return Version{}, err
	}
	return ParseVersion(out)
}

// Command is the shell line git runs on every push. It names the binary by
// absolute path, so PATH at push time doesn't matter, and it guards every
// failure: when the binary is gone, the shell's error goes to /dev/null and
// "|| :" turns the exit code into 0, so the push goes through.
func Command(exe string) string {
	return shellQuote(filepath.ToSlash(exe)) + " hook " + Event + " </dev/null >/dev/null 2>&1 || :"
}

// CommandPath reads back the binary a Command runs. ok is false for a
// command gbridge didn't write.
func CommandPath(cmd string) (string, bool) {
	p, rest, ok := shellUnquote(cmd)
	if !ok || !strings.HasPrefix(rest, " hook "+Event) {
		return "", false
	}
	return filepath.FromSlash(p), true
}

// shellQuote wraps s in single quotes, the one quoting where sh expands
// nothing.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellUnquote reads one word written by shellQuote from the start of s.
func shellUnquote(s string) (word, rest string, ok bool) {
	var b strings.Builder
	for strings.HasPrefix(s, "'") {
		end := strings.IndexByte(s[1:], '\'')
		if end < 0 {
			return "", "", false
		}
		b.WriteString(s[1 : 1+end])
		s = s[end+2:]
		if !strings.HasPrefix(s, `\'`) {
			return b.String(), s, true
		}
		b.WriteByte('\'')
		s = s[2:]
	}
	return "", "", false
}

// Global is the push hook as global git config holds it.
type Global struct {
	Command string   // hook.gbridge.command
	Events  []string // hook.gbridge.event
}

// Set reports whether any hook.gbridge key is in global config.
func (g Global) Set() bool { return g.Command != "" || len(g.Events) > 0 }

// Installed reports whether global config holds a complete push hook. Only
// Git 2.54 and later run it.
func (g Global) Installed() bool { return g.Command != "" && slices.Contains(g.Events, Event) }

// Exe is the binary the installed command runs.
func (g Global) Exe() (string, bool) { return CommandPath(g.Command) }

// ReadGlobal reads the push hook from global git config.
func ReadGlobal() (Global, error) {
	var g Global
	cmd, err := configGet("", "--global", "--get", "hook."+Name+".command")
	if err != nil {
		return g, err
	}
	g.Command = cmd
	ev, err := configGet("", "--global", "--get-all", "hook."+Name+".event")
	if err != nil {
		return g, err
	}
	if ev != "" {
		g.Events = strings.Split(ev, "\n")
	}
	return g, nil
}

// InstallGlobal writes the push hook into global git config, replacing any
// earlier one. The command goes first: an event without a command makes git
// abort commit, fetch, and status on the whole machine.
func InstallGlobal(exe string) error {
	if _, err := gitx.Git("", "config", "--global", "--replace-all", "hook."+Name+".command", Command(exe)); err != nil {
		return err
	}
	_, err := gitx.Git("", "config", "--global", "--replace-all", "hook."+Name+".event", Event)
	return err
}

// UninstallGlobal removes the push hook from global git config in one write,
// so no half is left behind. removed is false when there was nothing to
// remove.
func UninstallGlobal() (removed bool, err error) {
	out, err := configGet("", "--global", "--get-regexp", `^hook\.`+Name+`\.`)
	if err != nil || out == "" {
		return false, err
	}
	if _, err := gitx.Git("", "config", "--global", "--remove-section", "hook."+Name); err != nil {
		return false, err
	}
	return true, nil
}

// isFalse reads a git boolean the way git does for the false spellings.
// Anything else, including no value, is true.
func isFalse(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "no", "off", "0":
		return true
	}
	return false
}

// configGet runs git config in dir and returns its output. A key that isn't
// set is not an error: git config exits 1 for it, and configGet returns "".
func configGet(dir string, args ...string) (string, error) {
	out, err := gitx.Git(dir, append([]string{"config"}, args...)...)
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "", nil
	}
	return out, err
}

// ErrTemporaryBinary is returned for a binary go run built: it is deleted
// when go run exits, so a push hook naming it would point nowhere.
var ErrTemporaryBinary = errors.New("this gbridge is a temporary go run build; install it (go install, or a release) and run hook install from there")

// Executable is the absolute path the push hook should run: this binary,
// through its PATH entry when that is the same file, since a package
// manager keeps the PATH entry stable across upgrades while the real file
// moves.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if lp, err := exec.LookPath("gbridge"); err == nil {
		if abs, err := filepath.Abs(lp); err == nil && sameFile(abs, exe) {
			exe = abs
		}
	}
	return checkExecutable(exe)
}

func checkExecutable(exe string) (string, error) {
	if !filepath.IsAbs(exe) {
		return "", fmt.Errorf("binary path %q is not absolute", exe)
	}
	if strings.Contains(filepath.ToSlash(exe), "/go-build") {
		return "", ErrTemporaryBinary
	}
	return exe, nil
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

// Exists reports whether the binary a push hook runs is still there.
func Exists(exe string) bool {
	fi, err := os.Stat(exe)
	return err == nil && !fi.IsDir()
}
