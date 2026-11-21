package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden from the code")

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the output (go test -update rewrites it):\n got:\n%s\nwant:\n%s", name, got, want)
	}
}

// -h on a command without flags prints that command's help instead of
// reaching the command, which would read "--help" as a value.
func TestHelpFlag(t *testing.T) {
	for _, name := range []string{"add", "remove", "config"} {
		for _, h := range []string{"-h", "--help", "-help"} {
			var b bytes.Buffer
			if !printHelp(&b, name, []string{h}) {
				t.Errorf("gbridge %s %s: not handled as help", name, h)
				continue
			}
			golden(t, "help-"+name+".golden", b.String())
		}
	}
	// Help anywhere in the arguments: no email, path, or folder is "--help".
	var b bytes.Buffer
	if !printHelp(&b, "add", []string{"email", "--help"}) {
		t.Error("gbridge add email --help: not handled as help")
	}
}

func TestHelpFlagLeavesOtherArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"add", []string{"email", "me@work.example"}},
		{"remove", nil},
		{"config", nil},
		{"init", []string{"-h"}}, // the flag package prints init's help
		{"nope", []string{"-h"}},
	} {
		var b bytes.Buffer
		if printHelp(&b, tc.name, tc.args) || b.Len() > 0 {
			t.Errorf("gbridge %s %q: handled as help, printed %q", tc.name, tc.args, b.String())
		}
	}
}

// Completion offers words in the slot after a command, flags after the
// rest. The scripts can't do both for one command.
func TestCommandsWordsOrFlags(t *testing.T) {
	for _, c := range commands {
		if c.words != nil && c.flags != nil {
			t.Errorf("%s has both words and flags", c.name)
		}
	}
}
