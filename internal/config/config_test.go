package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateRejectsWhatGitHubCannotUse(t *testing.T) {
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string // substring of the error; empty means valid
	}{
		{"valid", func(*Config) {}, ""},
		{"local only", func(c *Config) { c.Mirror.Repo, c.Mirror.Remote = "", "" }, ""},
		{"dotted mirror name", func(c *Config) { c.Mirror.Repo = "octocat/my.activity_log-2" }, ""},
		{"author email without @", func(c *Config) { c.MirrorAuthor.Email = "octocat" }, "author.email"},
		{"author email with a display name", func(c *Config) { c.MirrorAuthor.Email = "Octo <me@example.com>" }, "author.email"},
		{"mirror name with a space", func(c *Config) { c.Mirror.Repo = "octocat/my mirror" }, "mirror.repo"},
		{"mirror without owner", func(c *Config) { c.Mirror.Repo = "greenbridge-activity" }, "mirror.repo"},
		{"mirror named ..", func(c *Config) { c.Mirror.Repo = "octocat/.." }, "mirror.repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Default()
			if err != nil {
				t.Fatal(err)
			}
			c.Emails = []string{"me@work.example"}
			c.MirrorAuthor = MirrorAuthor{Name: "octocat", Email: "583231+octocat@users.noreply.github.com"}
			c.Mirror.Repo = "octocat/greenbridge-activity"
			c.Mirror.Remote = "https://github.com/octocat/greenbridge-activity.git"
			tt.mutate(c)
			err = c.Validate()
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("Validate() = %v, want nil", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("Validate() = %v, want an error naming %s", err, tt.want)
			}
		})
	}
}

// Configs written before scan folders existed keep loading, and a config
// without scan folders saves without the field, so older gbridge builds read
// it unchanged.
func TestLoadConfigWithoutScanFolders(t *testing.T) {
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	p, _ := Path()
	old := `{"version":1,"emails":["me@work.example"],"repos":["/work/api"],
"mirror":{"path":"/m.git","branch":"main","visibility":"private"},
"author":{"name":"me","email":"me@example.com"},"time_policy":"exact","identity":"author-date"}`
	if err := os.WriteFile(p, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if len(c.ScanFolders) != 0 || !slices.Equal(c.Repos, []string{"/work/api"}) {
		t.Errorf("scan %q repos %q, want none and [/work/api]", c.ScanFolders, c.Repos)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); strings.Contains(string(b), `"scan"`) {
		t.Errorf("saved config gained a scan field:\n%s", b)
	}

	c.ScanFolders = []string{"/work"}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if c, err = Load(); err != nil || !slices.Equal(c.ScanFolders, []string{"/work"}) {
		t.Errorf("reloaded scan %q, %v; want [/work]", c.ScanFolders, err)
	}
}

// Sync records the repos it found under scan folders, so the next run can
// say which ones are new. A missing record means nothing was seen yet.
func TestSeen(t *testing.T) {
	t.Setenv("GBRIDGE_HOME", t.TempDir())
	if seen, err := LoadSeen(); err != nil || len(seen) != 0 {
		t.Fatalf("LoadSeen() on a fresh home = %v, %v; want empty", seen, err)
	}
	a, b := filepath.Join("work", "a"), filepath.Join("work", "b")
	if err := AddSeen([]string{a}); err != nil {
		t.Fatal(err)
	}
	if err := AddSeen([]string{b, a + string(filepath.Separator)}); err != nil {
		t.Fatal(err)
	}
	seen, err := LoadSeen()
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !seen[a] || !seen[b] {
		t.Errorf("seen = %v, want %s and %s", seen, a, b)
	}
}
