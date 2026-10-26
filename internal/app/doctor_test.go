package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sorfeb/greenbridge/internal/config"
)

// Each failure names its fix once: on the fix line, not also in the message.
func TestDoctorNamesInitOnce(t *testing.T) {
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no git, no gh: doctor stays off the network

	var out bytes.Buffer
	Doctor(&out)
	if n := strings.Count(out.String(), "run gbridge init"); n != 1 {
		t.Errorf("missing config: %q appears %d times, want 1:\n%s", "run gbridge init", n, out.String())
	}

	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Emails = []string{"me@work.example"}
	cfg.Author = config.Author{Name: "me", Email: "1+me@users.noreply.github.com"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	Doctor(&out)
	if !strings.Contains(out.String(), "no secret at") {
		t.Fatalf("missing secret not reported:\n%s", out.String())
	}
	if n := strings.Count(out.String(), "run gbridge init"); n != 1 {
		t.Errorf("missing secret: %q appears %d times, want 1:\n%s", "run gbridge init", n, out.String())
	}
}
