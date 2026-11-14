package app

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sorfeb/greenbridge/internal/config"
)

// Add puts emails, repos, or the repos found under scan folders into the
// config, as kind "email", "repo", or "scan" says. It writes only the config
// file: the secret and the mirror stay as they are, so mirror IDs don't
// change. Any bad value aborts the edit before anything is written.
func Add(kind string, values []string, out io.Writer) error {
	if len(values) == 0 {
		return fmt.Errorf("usage: gbridge add %s <value> [value ...]", kindOr(kind, "email|repo|scan"))
	}
	var change func(*config.Config) (bool, error)
	switch kind {
	case "email":
		change = func(c *config.Config) (bool, error) { return addEmails(out, c, values) }
	case "repo":
		change = func(c *config.Config) (bool, error) { return addRepos(out, c, values, nil) }
	case "scan":
		change = func(c *config.Config) (bool, error) { return addRepos(out, c, nil, values) }
	default:
		return fmt.Errorf("gbridge add %q: want email, repo, or scan", kind)
	}
	return editConfig(out, change)
}

// Remove takes emails or repos out of the config, as kind "email" or "repo"
// says. Commits already mirrored stay in the mirror.
func Remove(kind string, values []string, out io.Writer) error {
	if len(values) == 0 {
		return fmt.Errorf("usage: gbridge remove %s <value> [value ...]", kindOr(kind, "email|repo"))
	}
	var change func(*config.Config) (bool, error)
	switch kind {
	case "email":
		change = func(c *config.Config) (bool, error) { return removeEmails(out, c, values) }
	case "repo":
		change = func(c *config.Config) (bool, error) { return removeRepos(out, c, values) }
	default:
		return fmt.Errorf("gbridge remove %q: want email or repo", kind)
	}
	return editConfig(out, change)
}

func kindOr(kind, def string) string {
	if kind == "" {
		return def
	}
	return kind
}

// editConfig loads the config, applies change, and saves the result when
// change reports a difference. A change that fails writes nothing.
func editConfig(out io.Writer, change func(*config.Config) (bool, error)) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	changed, err := change(cfg)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprintln(out, "Config unchanged, nothing new to add.")
		return nil
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	path, _ := config.Path()
	fmt.Fprintf(out, "Saved %s\nNext: gbridge plan (preview), then gbridge sync.\n", path)
	return nil
}

func addEmails(out io.Writer, cfg *config.Config, emails []string) (bool, error) {
	var added []string
	for _, e := range emails {
		e = strings.TrimSpace(e)
		local, domain, ok := strings.Cut(e, "@")
		if !ok || local == "" || domain == "" || strings.ContainsAny(e, " <>") {
			return false, fmt.Errorf("%q is not an email address", e)
		}
		if hasEmail(cfg.Emails, e) || hasEmail(added, e) {
			fmt.Fprintf(out, "  have  %s (already in config)\n", e)
			continue
		}
		fmt.Fprintf(out, "  add   %s\n", e)
		added = append(added, e)
	}
	cfg.Emails = append(cfg.Emails, added...)
	return len(added) > 0, nil
}

func hasEmail(list []string, e string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, e) })
}

// addRepos adds the given repos, which must all qualify, and the repos found
// under the scan folders that have commits by cfg's emails.
func addRepos(out io.Writer, cfg *config.Config, paths, scan []string) (bool, error) {
	candidates := repoCandidates(paths, scan)
	if len(candidates) == 0 {
		return false, fmt.Errorf("no git repos under %s", strings.Join(scan, ", "))
	}
	added, refused := pickRepos(out, cfg, candidates, paths)
	if len(refused) > 0 {
		return false, fmt.Errorf("not added, see above: %s", strings.Join(refused, ", "))
	}
	cfg.Repos = append(cfg.Repos, added...)
	return len(added) > 0, nil
}

func removeEmails(out io.Writer, cfg *config.Config, emails []string) (bool, error) {
	for _, e := range emails {
		e = strings.TrimSpace(e)
		i := slices.IndexFunc(cfg.Emails, func(x string) bool { return strings.EqualFold(x, e) })
		if i < 0 {
			return false, fmt.Errorf("%s is not in config emails", e)
		}
		fmt.Fprintf(out, "  remove  %s\n", cfg.Emails[i])
		cfg.Emails = slices.Delete(cfg.Emails, i, i+1)
	}
	if len(cfg.Emails) == 0 {
		return false, errors.New("config needs at least one email: gbridge add email <address> first, then remove the old one")
	}
	return true, nil
}

// removeRepos matches each path as given or made absolute, so a clone that
// no longer exists can still be removed.
func removeRepos(out io.Writer, cfg *config.Config, paths []string) (bool, error) {
	for _, p := range paths {
		i := slices.IndexFunc(cfg.Repos, func(r string) bool { return r == p || r == absPath(p) })
		if i < 0 {
			return false, fmt.Errorf("%s is not in config repos (gbridge config lists them)", p)
		}
		fmt.Fprintf(out, "  remove  %s\n", cfg.Repos[i])
		cfg.Repos = slices.Delete(cfg.Repos, i, i+1)
	}
	fmt.Fprintln(out, "Commits already mirrored from removed repos stay in the mirror.")
	return true, nil
}

// ShowConfig prints the current setup. It never prints the secret.
func ShowConfig(out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	path, _ := config.Path()
	writeConfig(out, path, cfg)
	return nil
}

func writeConfig(w io.Writer, path string, cfg *config.Config) {
	line := func(label, value string) { fmt.Fprintf(w, "%-12s %s\n", label, value) }
	list := func(label string, values []string, none string) {
		if len(values) == 0 {
			line(label, none)
		}
		for i, v := range values {
			if i > 0 {
				label = ""
			}
			line(label, v)
		}
	}
	line("config", path)
	list("emails", cfg.Emails, "none (gbridge add email <address>)")
	list("repos", cfg.Repos, "none (gbridge add repo <path> or gbridge add scan <folder>)")
	if cfg.IncludeGitHubRepos {
		line("", "repos with a github.com remote included (counts twice)")
	}
	switch {
	case cfg.Mirror.LocalOnly():
		line("mirror", "local only, nothing reaches GitHub")
	case cfg.Mirror.Repo != "":
		line("mirror", cfg.Mirror.Repo+", "+cfg.Mirror.Visibility+", branch "+cfg.Mirror.Branch)
	default:
		line("mirror", cfg.Mirror.Remote+", "+cfg.Mirror.Visibility+", branch "+cfg.Mirror.Branch)
	}
	line("", "local clone "+cfg.Mirror.Path)
	line("timestamps", cfg.TimePolicy)
	line("author", cfg.Author.Name+" <"+cfg.Author.Email+">")
}
