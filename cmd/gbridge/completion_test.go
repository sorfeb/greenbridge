package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// script writes the completion script for shell to a temp file.
func script(t *testing.T, shell string) string {
	t.Helper()
	var b bytes.Buffer
	if err := runCompletion(&b, []string{shell}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "gbridge."+shell)
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// shellPath finds a shell or skips. On Windows, System32's bash is WSL,
// which can't read Windows temp paths.
func shellPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not installed", name)
	}
	if runtime.GOOS == "windows" && strings.Contains(strings.ToLower(p), `\system32\`) {
		t.Skipf("%s at %s is WSL", name, p)
	}
	return p
}

func lines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

func TestBashCompletion(t *testing.T) {
	bash := shellPath(t, "bash")
	src := script(t, "bash")
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sum.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		words []string
		want  []string
	}{
		{[]string{"gbridge", "sy"}, []string{"sync"}},
		{[]string{"gbridge", "c"}, []string{"completion", "config"}},
		{[]string{"gbridge", "init", "--sc"}, []string{"--scan"}},
		{[]string{"gbridge", "plan", "--"}, []string{"--fetch", "--offline", "--since", "--weeks"}},
		{[]string{"gbridge", "backfill", "--d"}, []string{"--detach", "--dry-run"}},
		{[]string{"gbridge", "completion", "p"}, []string{"powershell"}},
		{[]string{"gbridge", "init", "--scan", "su"}, []string{"sub"}}, // directories only, not sum.txt
		{[]string{"gbridge", "init", "--email", ""}, nil},              // a value, nothing to offer
		{[]string{"gbridge", "doctor", "--"}, nil},
		{[]string{"gbridge", "add", ""}, []string{"email", "repo", "scan"}},
		{[]string{"gbridge", "remove", "r"}, []string{"repo"}},
		{[]string{"gbridge", "add", "email", ""}, nil}, // the kind is given
		{[]string{"gbridge", "completion", "zsh", ""}, nil},
	}
	for _, c := range cases {
		cmd := exec.Command(bash, append([]string{
			"-c",
			`source "$1"; shift; COMP_WORDS=("$@"); COMP_CWORD=$(($#-1)); _gbridge; printf '%s\n' "${COMPREPLY[@]}"`,
			"bash", filepath.ToSlash(src),
		}, c.words...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", c.words, err, out)
		}
		if got := lines(string(out)); !slices.Equal(got, c.want) {
			t.Errorf("%v: got %q, want %q", c.words, got, c.want)
		}
	}
}

func TestPowerShellCompletion(t *testing.T) {
	pwsh := shellPath(t, "pwsh")
	src := script(t, "powershell")
	cases := []struct {
		input string
		want  []string
	}{
		{"gbridge sy", []string{"sync"}},
		{"gbridge init --sc", []string{"--scan"}},
		{"gbridge plan --", []string{"--fetch", "--offline", "--since", "--weeks"}},
		{"gbridge completion f", []string{"fish"}},
		{"gbridge add r", []string{"repo"}},
		{"gbridge remove s", []string{"scan"}},
	}
	var ps strings.Builder
	ps.WriteString("$ErrorActionPreference = 'Stop'\nGet-Content -Raw '" + src + "' | Invoke-Expression\n")
	for _, c := range cases {
		ps.WriteString("(TabExpansion2 -inputScript '" + c.input + "' -cursorColumn " +
			strconv.Itoa(len(c.input)) + ").CompletionMatches.CompletionText -join ' '\n")
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", ps.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	got := strings.Split(strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n")
	if len(got) != len(cases) {
		t.Fatalf("got %d result lines, want %d:\n%s", len(got), len(cases), out)
	}
	for i, c := range cases {
		g := strings.Fields(got[i])
		slices.Sort(g)
		if !slices.Equal(g, c.want) {
			t.Errorf("%q: got %q, want %q", c.input, g, c.want)
		}
	}
}

func TestFishCompletion(t *testing.T) {
	fish := shellPath(t, "fish")
	src := script(t, "fish")
	cases := []struct {
		input string
		want  []string
	}{
		{"gbridge sy", []string{"sync"}},
		{"gbridge add ", []string{"email", "repo", "scan"}},
		{"gbridge remove e", []string{"email"}},
		{"gbridge add email ", nil}, // the kind is given
		{"gbridge completion ", []string{"bash", "fish", "powershell", "zsh"}},
		{"gbridge completion bash ", nil},
		{"gbridge init --sca", []string{"--scan"}}, // fish matches fuzzily: --sc also finds --secret
	}
	for _, c := range cases {
		out, err := exec.Command(fish, "--no-config", "-c", "source $argv[1]; complete -C $argv[2]",
			src, c.input).CombinedOutput()
		if err != nil {
			t.Fatalf("%q: %v\n%s", c.input, err, out)
		}
		var got []string
		for _, l := range lines(string(out)) {
			got = append(got, strings.Fields(l)[0])
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%q: got %q, want %q", c.input, got, c.want)
		}
	}
}

func TestZshAndFishParse(t *testing.T) {
	for _, s := range []struct{ shell, bin string }{{"zsh", "zsh"}, {"fish", "fish"}} {
		t.Run(s.shell, func(t *testing.T) {
			bin := shellPath(t, s.bin)
			flag := "-n"
			if s.shell == "fish" {
				flag = "--no-execute"
			}
			if out, err := exec.Command(bin, flag, script(t, s.shell)).CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	err := runCompletion(&bytes.Buffer{}, []string{"tcsh"})
	if err == nil || !strings.Contains(err.Error(), "bash, zsh, fish, powershell") {
		t.Fatalf("got %v, want an error naming the supported shells", err)
	}
}
