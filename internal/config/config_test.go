package config

import (
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
		{"author email without @", func(c *Config) { c.Author.Email = "octocat" }, "author.email"},
		{"author email with a display name", func(c *Config) { c.Author.Email = "Octo <me@example.com>" }, "author.email"},
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
			c.Author = Author{Name: "octocat", Email: "583231+octocat@users.noreply.github.com"}
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
