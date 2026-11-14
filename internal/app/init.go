package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/gitx"
	"github.com/sorfeb/greenbridge/internal/ident"
)

// InitOptions are gbridge init flags. Anything left empty is asked for, unless
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

var errAborted = errors.New("aborted")

// Init asks for everything first, then writes it all in one apply step, so a
// failed or aborted init leaves no secret, config, or GitHub repo behind.
func Init(o InitOptions, gh ghx.GitHub, in io.Reader, out io.Writer) error {
	if p, _ := config.Path(); !o.Force {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("config exists at %s: change emails and repos with gbridge add or gbridge remove, or rerun with --force to replace it", p)
		}
	}
	cfg, err := config.Default()
	if err != nil {
		return err
	}
	p := newPrompter(in, out, o.Yes)

	fmt.Fprintln(out, employerNotice)
	if !p.confirm("Continue?") {
		return errAborted
	}
	cfg.NoticeAccepted = true
	cfg.IncludeGitHubRepos = o.IncludeGitHub

	d := &draft{cfg: cfg}
	if err := askEmails(p, cfg, o.Emails); err != nil {
		return err
	}
	if d.seen, err = askRepos(p, cfg, o.Repos, o.Scan); err != nil {
		return err
	}
	if err := d.decideSecret(out, o.Secret, o.Force); err != nil {
		return err
	}
	if err := d.askMirror(p, gh, o); err != nil {
		return err
	}
	if err := d.apply(gh, out); err != nil {
		return err
	}

	path, _ := config.Path()
	fmt.Fprintf(out, "\n%s\n\nSaved %s\n", leavesMachine, path)
	if cfg.Mirror.Visibility == "private" && !cfg.Mirror.LocalOnly() {
		fmt.Fprintf(out, "Private mirror: turn on private contributions at %s\n", PrivateContribSettings)
	}
	fmt.Fprintln(out, "Next: gbridge plan (preview), then gbridge sync.")
	return nil
}

// prompter asks questions on a reader and a writer. With yes set, every
// question takes its default without asking.
type prompter struct {
	rd  *bufio.Reader
	out io.Writer
	yes bool
}

func newPrompter(in io.Reader, out io.Writer, yes bool) *prompter {
	return &prompter{rd: bufio.NewReader(in), out: out, yes: yes}
}

func (p *prompter) ask(q, def string) string {
	if p.yes {
		return def
	}
	fmt.Fprintf(p.out, "%s [%s]: ", q, def)
	line, _ := p.rd.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return def
	}
	return line
}

func (p *prompter) confirm(q string) bool {
	a := strings.ToLower(p.ask(q+" (y/n)", "y"))
	return a == "y" || a == "yes"
}

// askEmails sets the work emails: given, else asked with the current ones
// (or git's user.email) as the default.
func askEmails(p *prompter, cfg *config.Config, given []string) error {
	emails := given
	if len(emails) == 0 {
		def := strings.Join(cfg.Emails, ", ")
		if def == "" {
			def, _ = gitx.Git("", "config", "--global", "user.email")
		}
		emails = splitList(p.ask("Work emails to match, comma separated", def))
	}
	if len(emails) == 0 {
		return errors.New("at least one email is required")
	}
	cfg.Emails = emails
	return nil
}

// askRepos adds the given paths and scan folders to cfg (folders asked for
// when both are empty), shows which repos count, and returns the repos the
// scan folders hold now. A named repo is kept without commits by cfg's
// emails; a found one needs them. A scan folder is kept even when it holds
// none yet, since plan and sync search it again every run.
func askRepos(p *prompter, cfg *config.Config, paths, scan []string) (found []string, err error) {
	if len(paths) == 0 && len(scan) == 0 {
		scan = splitList(p.ask("Folders to scan for work repos, comma separated", ""))
	}
	var dirs []string
	var repos []sourceRepo
	for _, d := range scan {
		d = absPath(d)
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			fmt.Fprintf(p.out, "  skip  %s (not a folder)\n", d)
			continue
		}
		if !slices.Contains(dirs, d) {
			dirs, repos = append(dirs, d), append(repos, findRepos(d)...)
		}
	}
	added, found, _ := pickRepos(p.out, cfg, paths, repos)
	if len(added) == 0 && len(dirs) == 0 {
		fmt.Fprintln(p.out, "No source repos yet. Add them later with gbridge add repo <path> or gbridge add scan <folder>.")
		return nil, nil
	}
	q := fmt.Sprintf("Use these %d repos?", len(added)+len(found))
	if len(dirs) > 0 {
		q = fmt.Sprintf("Use these %d repos, and search %s for new ones on every sync?", len(added)+len(found), strings.Join(dirs, ", "))
	}
	if !p.confirm(q) {
		return nil, errAborted
	}
	cfg.Repos = append(cfg.Repos, added...)
	cfg.Scan = append(cfg.Scan, dirs...)
	return found, nil
}

// pickRepos judges the repos the user named (paths) and the repos found in
// scan folders against cfg, says why on out, and sorts them: named repos to
// add to cfg.Repos, found repos worth mirroring, and named repos refused, all
// as absolute paths. Repos already in cfg.Repos are left alone. It reads each
// repo the way plan does.
func pickRepos(out io.Writer, cfg *config.Config, paths []string, found []sourceRepo) (added, kept, refused []string) {
	have := map[string]bool{}
	for _, r := range cfg.Repos {
		have[filepath.Clean(r)] = true
	}
	var repos []sourceRepo
	listed := map[string]bool{}
	for _, r := range append(namedRepos(paths), found...) {
		if !listed[r.path] {
			listed[r.path] = true
			repos = append(repos, r)
		}
	}
	ins := inspectAll(cfg, repos, time.Time{})
	for i, r := range repos {
		named := r.scan == ""
		switch {
		case have[r.path]:
			fmt.Fprintf(out, "  have  %s (already in config)\n", r.path)
		case !keepRepo(out, r.path, named, ins[i]):
			if named {
				refused = append(refused, r.path)
			}
		case named:
			added = append(added, r.path)
		default:
			kept = append(kept, r.path)
		}
	}
	return added, kept, refused
}

func namedRepos(paths []string) []sourceRepo {
	out := make([]sourceRepo, 0, len(paths))
	for _, p := range paths {
		out = append(out, sourceRepo{path: absPath(p)})
	}
	return out
}

// keepRepo reports whether repo r is worth mirroring, given what inspecting
// it found, and says why on out.
func keepRepo(out io.Writer, r string, named bool, in inspection) bool {
	switch {
	case in.err != nil:
		fmt.Fprintf(out, "  skip  %s (%v)\n", r, in.err)
		return false
	case in.hosted:
		fmt.Fprintf(out, "  skip  %s (github.com remote, GitHub already counts it; --include-github to override)\n", r)
		return false
	case len(in.acts) == 0 && !named:
		fmt.Fprintf(out, "  skip  %s (no commits by these emails)\n", r)
		return false
	case named:
		fmt.Fprintf(out, "  add   %s (%d commits)\n", r, len(in.acts))
	default:
		fmt.Fprintf(out, "  found %s (%d commits)\n", r, len(in.acts))
	}
	return true
}

// draft is a config init has decided but not written, and the writes that
// go with it. apply carries them out.
type draft struct {
	cfg      *config.Config
	secret   []byte   // to write; nil keeps the secret on disk
	imported bool     // secret came from a recovery phrase, so there is none to show
	create   string   // mirror repo name to create on GitHub; empty creates none
	seen     []string // repos init showed from scan folders, recorded so plan does not call them new
}

// decideSecret picks the secret: the given recovery phrase, else the one on
// disk, else a new one. It never replaces a different secret without force,
// since a new secret changes every mirror ID and the next sync double counts.
func (d *draft) decideSecret(out io.Writer, phrase string, force bool) error {
	sp, err := config.SecretPath()
	if err != nil {
		return err
	}
	existing, loadErr := ident.Load(sp)
	switch {
	case phrase != "":
		s, err := ident.Decode(phrase)
		if err != nil {
			return err
		}
		if loadErr == nil && !bytes.Equal(s, existing) && !force {
			return errors.New("a different secret already exists: rerun with --force to replace it (mirror IDs will change)")
		}
		d.secret, d.imported = s, true
	case loadErr == nil:
		fmt.Fprintln(out, "Keeping existing secret.")
	default:
		s, err := ident.NewSecret()
		if err != nil {
			return err
		}
		d.secret = s
	}
	return nil
}

// askMirror sets the mirror author, visibility and repo. It only reads from
// GitHub; a repo that does not exist yet is left for apply to create.
func (d *draft) askMirror(p *prompter, gh ghx.GitHub, o InitOptions) error {
	cfg := d.cfg
	cfg.Author = config.Author{Name: o.AuthorName, Email: o.AuthorEmail}
	user, ghErr := gh.Account()
	haveGH := ghErr == nil
	if cfg.Author.Email == "" {
		if !haveGH {
			return errors.New("gh is not logged in: run gh auth login, or pass --author-email")
		}
		cfg.Author.Email = user.NoreplyEmail
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
	if o.LocalOnly {
		return nil
	}
	if !haveGH {
		return errors.New("gh is not logged in: run gh auth login, or pass --local-only")
	}
	name := o.MirrorName
	if name == "" {
		name = p.ask("Mirror repo name", "greenbridge-activity")
	}
	cfg.Mirror.Repo = user.Login + "/" + name
	cfg.Mirror.Remote = "https://github.com/" + cfg.Mirror.Repo + ".git"
	if _, err := gh.Repo(cfg.Mirror.Repo); err == nil {
		fmt.Fprintf(p.out, "Using existing %s\n", cfg.Mirror.Repo)
		return nil
	}
	if !p.confirm(fmt.Sprintf("Create %s repo %s?", cfg.Mirror.Visibility, cfg.Mirror.Repo)) {
		return errAborted
	}
	d.create = name
	return nil
}

// apply validates the draft, then writes the secret, creates the GitHub repo,
// saves the config and the repos seen in scan folders, and ensures the local
// mirror, in that order. An invalid draft writes nothing.
func (d *draft) apply(gh ghx.GitHub, out io.Writer) error {
	if err := d.cfg.Validate(); err != nil {
		return err
	}
	if d.secret != nil {
		if err := d.saveSecret(out); err != nil {
			return err
		}
	}
	if d.create != "" {
		if err := gh.CreateRepo(d.create, d.cfg.Mirror.Visibility == "private"); err != nil {
			return err
		}
		fmt.Fprintf(out, "Created %s\n", d.cfg.Mirror.Repo)
	}
	if err := d.cfg.Save(); err != nil {
		return err
	}
	if err := config.AddSeen(d.seen); err != nil {
		return err
	}
	return MirrorOf(d.cfg).Ensure()
}

func (d *draft) saveSecret(out io.Writer) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	sp, err := config.SecretPath()
	if err != nil {
		return err
	}
	if err := ident.Save(sp, d.secret); err != nil {
		return err
	}
	if d.imported {
		fmt.Fprintln(out, "Secret imported.")
	} else {
		fmt.Fprintf(out, "\nRecovery phrase (store it in your password manager; use gbridge init --secret on other machines):\n  %s\n\n", ident.Encode(d.secret))
	}
	return nil
}

func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
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
