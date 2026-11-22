package app

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
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
