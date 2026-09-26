package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/gitx"
	"github.com/sorfeb/greenbridge/internal/ident"
	"github.com/sorfeb/greenbridge/internal/source"
)

// InitOptions are gb init flags. Anything left empty is asked for, unless
// Yes is set, in which case defaults apply.
type InitOptions struct {
	Emails        []string
	Repos         []string
	Scan          []string
	IncludeGitHub bool
	MirrorName    string
	LocalOnly     bool
	Public        bool
	Secret        string
	AuthorName    string
	AuthorEmail   string
	Yes           bool
	Force         bool
}

const employerNotice = `Before you continue: mirroring work activity, even as timestamps only,
may be covered by your employer's policies or contract. Check them first.
greenbridge never copies code, file names, messages, or repo names.`

const leavesMachine = `What leaves this machine: one empty commit per source commit, carrying
a timestamp (or date only, for public mirrors) and an opaque 12-character ID.
What stays: code, diffs, file names, commit messages, repo and branch names,
commit hashes, and your work email.`

// Init creates the config and secret, and the mirror repo on GitHub.
func Init(o InitOptions, in io.Reader, out io.Writer) error {
	rd := bufio.NewReader(in)
	ask := func(q, def string) string {
		if o.Yes {
			return def
		}
		fmt.Fprintf(out, "%s [%s]: ", q, def)
		line, _ := rd.ReadString('\n')
		if line = strings.TrimSpace(line); line == "" {
			return def
		}
		return line
	}
	confirm := func(q string) bool {
		a := strings.ToLower(ask(q+" (y/n)", "y"))
		return a == "y" || a == "yes"
	}

	if p, _ := config.Path(); !o.Force {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("config exists at %s: rerun with --force to replace it", p)
		}
	}
	cfg, err := config.Default()
	if err != nil {
		return err
	}

	fmt.Fprintln(out, employerNotice)
	if !confirm("Continue?") {
		return errors.New("aborted")
	}
	cfg.NoticeAccepted = true

	// Emails.
	emails := o.Emails
	if len(emails) == 0 {
		def, _ := gitx.Git("", "config", "--global", "user.email")
		emails = splitList(ask("Work emails to match, comma separated", def))
	}
	if len(emails) == 0 {
		return errors.New("at least one email is required")
	}
	cfg.Emails = emails
	cfg.IncludeGitHubRepos = o.IncludeGitHub

	// Repos.
	candidates := append([]string{}, o.Repos...)
	for _, dir := range o.Scan {
		candidates = append(candidates, source.FindRepos(dir, 4)...)
	}
	if len(candidates) == 0 && !o.Yes {
		for _, d := range splitList(ask("Folders to scan for work repos, comma separated", "")) {
			candidates = append(candidates, source.FindRepos(d, 4)...)
		}
	}
	explicit := map[string]bool{}
	for _, r := range o.Repos {
		explicit[absPath(r)] = true
	}
	seen := map[string]bool{}
	for _, r := range candidates {
		r = absPath(r)
		if seen[r] {
			continue
		}
		seen[r] = true
		acts, err := source.Scan(r, emails, time.Time{})
		if err != nil {
			fmt.Fprintf(out, "  skip  %s (%v)\n", r, err)
			continue
		}
		gh, _ := source.HasGitHubRemote(r)
		switch {
		case gh && !o.IncludeGitHub:
			fmt.Fprintf(out, "  skip  %s (github.com remote, GitHub already counts it; --include-github to override)\n", r)
		case len(acts) == 0 && !explicit[r]:
			fmt.Fprintf(out, "  skip  %s (no commits by these emails)\n", r)
		default:
			fmt.Fprintf(out, "  add   %s (%d commits)\n", r, len(acts))
			cfg.Repos = append(cfg.Repos, r)
		}
	}
	if len(cfg.Repos) == 0 {
		fmt.Fprintln(out, "No source repos yet. Add paths to \"repos\" in the config later.")
	} else if !confirm(fmt.Sprintf("Use these %d repos?", len(cfg.Repos))) {
		return errors.New("aborted")
	}

	// Secret: never silently replaced, since a new secret double counts.
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	sp, _ := config.SecretPath()
	existing, loadErr := ident.Load(sp)
	switch {
	case o.Secret != "":
		s, err := ident.Decode(o.Secret)
		if err != nil {
			return err
		}
		if loadErr == nil && !bytes.Equal(s, existing) && !o.Force {
			return errors.New("a different secret already exists: rerun with --force to replace it (mirror IDs will change)")
		}
		if err := ident.Save(sp, s); err != nil {
			return err
		}
		fmt.Fprintln(out, "Secret imported.")
	case loadErr == nil:
		fmt.Fprintln(out, "Keeping existing secret.")
	default:
		s, err := ident.NewSecret()
		if err != nil {
			return err
		}
		if err := ident.Save(sp, s); err != nil {
			return err
		}
		fmt.Fprintf(out, "\nRecovery phrase (store it in your password manager; use gb init --secret on other machines):\n  %s\n\n", ident.Encode(s))
	}

	// Author and mirror.
	cfg.Author = config.Author{Name: o.AuthorName, Email: o.AuthorEmail}
	var user ghx.User
	haveGH := false
	if ghx.Available() {
		if u, err := ghx.CurrentUser(); err == nil {
			user, haveGH = u, true
		}
	}
	if cfg.Author.Email == "" {
		if !haveGH {
			return errors.New("gh is not logged in: run gh auth login, or pass --author-email")
		}
		cfg.Author.Email = user.NoreplyEmail()
	}
	if cfg.Author.Name == "" {
		cfg.Author.Name = user.Login
		if cfg.Author.Name == "" {
			cfg.Author.Name = "greenbridge"
		}
	}
	if o.Public {
		cfg.Mirror.Visibility = "public"
		cfg.TimePolicy = ident.PolicyDateOnly
	}
	if !o.LocalOnly {
		if !haveGH {
			return errors.New("gh is not logged in: run gh auth login, or pass --local-only")
		}
		name := o.MirrorName
		if name == "" {
			name = ask("Mirror repo name", "greenbridge-activity")
		}
		cfg.Mirror.Repo = user.Login + "/" + name
		cfg.Mirror.Remote = "https://github.com/" + cfg.Mirror.Repo + ".git"
		if _, err := ghx.ViewRepo(cfg.Mirror.Repo); err != nil {
			if !confirm(fmt.Sprintf("Create %s repo %s?", cfg.Mirror.Visibility, cfg.Mirror.Repo)) {
				return errors.New("aborted")
			}
			if err := ghx.CreateRepo(name, !o.Public); err != nil {
				return err
			}
			fmt.Fprintf(out, "Created %s\n", cfg.Mirror.Repo)
		} else {
			fmt.Fprintf(out, "Using existing %s\n", cfg.Mirror.Repo)
		}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if err := MirrorOf(cfg).Ensure(); err != nil {
		return err
	}
	p, _ := config.Path()
	fmt.Fprintf(out, "\n%s\n\nSaved %s\n", leavesMachine, p)
	if cfg.Mirror.Visibility == "private" && !o.LocalOnly {
		fmt.Fprintf(out, "Private mirror: turn on private contributions at %s\n", PrivateContribSettings)
	}
	fmt.Fprintln(out, "Next: gb plan (preview), then gb sync.")
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
