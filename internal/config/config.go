// Package config loads and saves greenbridge settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sorfeb/greenbridge/internal/ident"
)

// Mirror describes the activity repo that receives empty commits.
type Mirror struct {
	Repo       string `json:"repo,omitempty"`   // owner/name on GitHub; empty for local only
	Remote     string `json:"remote,omitempty"` // push URL; empty for local only
	Path       string `json:"path"`             // local bare clone
	Branch     string `json:"branch"`
	Visibility string `json:"visibility"` // private or public
}

// Author is the identity written on mirror commits.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Config is the on-disk configuration.
type Config struct {
	Version            int      `json:"version"`
	Emails             []string `json:"emails"`
	Repos              []string `json:"repos"`
	IncludeGitHubRepos bool     `json:"include_github_repos"`
	Mirror             Mirror   `json:"mirror"`
	Author             Author   `json:"author"`
	TimePolicy         string   `json:"time_policy"`
	Identity           string   `json:"identity"`
	NoticeAccepted     bool     `json:"notice_accepted"`
}

// ErrNoConfig is wrapped by Load when the config file does not exist.
var ErrNoConfig = errors.New("run gbridge init")

// Dir is the greenbridge home: $GBRIDGE_HOME, else <user config dir>/greenbridge.
func Dir() (string, error) {
	if d := os.Getenv("GBRIDGE_HOME"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "greenbridge"), nil
}

// Path is the config file location.
func Path() (string, error) {
	d, err := Dir()
	return filepath.Join(d, "config.json"), err
}

// SecretPath is the secret file location.
func SecretPath() (string, error) {
	d, err := Dir()
	return filepath.Join(d, "secret"), err
}

// Default returns a config with defaults filled in.
func Default() (*Config, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	return &Config{
		Version:    1,
		Mirror:     Mirror{Path: filepath.Join(d, "mirror.git"), Branch: "main", Visibility: "private"},
		TimePolicy: ident.PolicyExact,
		Identity:   ident.ModeAuthorDate,
	}, nil
}

// Load reads the config file.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no config at %s: %w", p, ErrNoConfig)
	}
	if err != nil {
		return nil, err
	}
	c, err := Default()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return c, c.Validate()
}

// Save writes the config file.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}

// Validate checks required fields and enum values.
func (c *Config) Validate() error {
	switch {
	case len(c.Emails) == 0:
		return errors.New("config: emails is empty")
	case c.Author.Email == "":
		return errors.New("config: author.email is empty")
	case c.Mirror.Path == "" || c.Mirror.Branch == "":
		return errors.New("config: mirror.path and mirror.branch are required")
	}
	if c.TimePolicy != ident.PolicyExact && c.TimePolicy != ident.PolicyDateOnly {
		return fmt.Errorf("config: time_policy %q: want exact or date-only", c.TimePolicy)
	}
	if c.Identity != ident.ModeAuthorDate && c.Identity != ident.ModeSHA {
		return fmt.Errorf("config: identity %q: want author-date or sha", c.Identity)
	}
	if c.Mirror.Visibility != "private" && c.Mirror.Visibility != "public" {
		return fmt.Errorf("config: mirror.visibility %q: want private or public", c.Mirror.Visibility)
	}
	return nil
}
