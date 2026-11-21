package hook

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/gittest"
)

func TestParseVersion(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Version
	}{
		{"git version 2.54.0.windows.1", Version{2, 54, 0}},
		{"git version 2.51.0", Version{2, 51, 0}},
		{"git version 2.39.3 (Apple Git-146)", Version{2, 39, 3}},
		{"git version 2.56.1\n", Version{2, 56, 1}},
		{"git version 3.0", Version{3, 0, 0}},
	} {
		got, err := ParseVersion(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "git version", "hub version 2.54.0", "git version x.y"} {
		if v, err := ParseVersion(bad); err == nil {
			t.Errorf("ParseVersion(%q) = %v, want an error", bad, v)
		}
	}
}

func TestConfigHooks(t *testing.T) {
	for _, tc := range []struct {
		v    Version
		want bool
	}{
		{Version{2, 51, 0}, false},
		{Version{2, 53, 9}, false},
		{Version{2, 54, 0}, true},
		{Version{2, 56, 1}, true},
		{Version{3, 0, 0}, true},
	} {
		if got := tc.v.ConfigHooks(); got != tc.want {
			t.Errorf("%v.ConfigHooks() = %v, want %v", tc.v, got, tc.want)
		}
	}
}

// The path the command runs is read back from it, whatever the path holds.
func TestCommandPath(t *testing.T) {
	for _, exe := range []string{
		`/usr/local/bin/gbridge`,
		`/home/o'brien/go/bin/gbridge`,
		`/opt/a "b" $HOME/gbridge`,
		`C:/Program Files/gbridge/gbridge.exe`,
	} {
		exe = filepath.FromSlash(exe)
		got, ok := CommandPath(Command(exe))
		if !ok || got != exe {
			t.Errorf("CommandPath(Command(%q)) = %q, %v", exe, got, ok)
		}
	}
	if p, ok := CommandPath("gbridge hook pre-push"); ok {
		t.Errorf("CommandPath of a foreign command = %q, want not ok", p)
	}
}

// isolate points git at an empty global config and away from the system
// config, so no test reads or writes the machine's own.
func isolate(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", p)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return p
}

func TestGlobalInstallUninstall(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	exe := filepath.Join(t.TempDir(), "Program Files", "gbridge.exe")

	g, err := ReadGlobal()
	if err != nil || g.Installed() || g.Set() {
		t.Fatalf("fresh config: %+v, %v; want nothing set", g, err)
	}
	if err := InstallGlobal(exe); err != nil {
		t.Fatal(err)
	}
	g, err = ReadGlobal()
	if err != nil || !g.Installed() {
		t.Fatalf("after install: %+v, %v; want installed", g, err)
	}
	if p, ok := g.Exe(); !ok || p != exe {
		t.Errorf("installed hook runs %q, %v; want %q", p, ok, exe)
	}
	// Installing again replaces, never adds a second event or command.
	if err := InstallGlobal(exe); err != nil {
		t.Fatal(err)
	}
	if ev := gittest.Git(t, "", nil, "config", "--global", "--get-all", "hook.gbridge.event"); ev != "pre-push" {
		t.Errorf("events after a second install = %q, want one pre-push", ev)
	}

	removed, err := UninstallGlobal()
	if err != nil || !removed {
		t.Fatalf("uninstall = %v, %v; want removed", removed, err)
	}
	if g, _ := ReadGlobal(); g.Set() {
		t.Errorf("after uninstall: %+v, want nothing set", g)
	}
	removed, err = UninstallGlobal()
	if err != nil || removed {
		t.Errorf("second uninstall = %v, %v; want nothing removed, no error", removed, err)
	}
}

// A command left without its event makes Git abort commit, fetch, and
// status on the whole machine; the reverse is harmless. ReadGlobal reports
// the half-written state so install can repair it.
func TestGlobalHalfWritten(t *testing.T) {
	gittest.Need(t)
	isolate(t)
	gittest.Git(t, "", nil, "config", "--global", "hook.gbridge.command", Command("/x/gbridge"))
	g, err := ReadGlobal()
	if err != nil || g.Installed() || !g.Set() {
		t.Errorf("command without event: %+v, %v; want set but not installed", g, err)
	}
}

func TestExecutableRefusesGoRun(t *testing.T) {
	_, err := checkExecutable(filepath.Join(os.TempDir(), "go-build123", "b001", "exe", "gbridge"))
	if !errors.Is(err, ErrTemporaryBinary) {
		t.Errorf("go run binary: err = %v, want ErrTemporaryBinary", err)
	}
	if _, err := checkExecutable("gbridge"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("relative path: err = %v, want an absolute-path error", err)
	}
}
