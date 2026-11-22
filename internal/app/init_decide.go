package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/ident"
)

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
	cfg.MirrorAuthor = config.MirrorAuthor{Name: o.AuthorName, Email: o.AuthorEmail}
	user, ghErr := gh.Account()
	haveGH := ghErr == nil
	if cfg.MirrorAuthor.Email == "" {
		if !haveGH {
			return errors.New("gh is not logged in: run gh auth login, or pass --author-email")
		}
		cfg.MirrorAuthor.Email = user.NoreplyEmail
	}
	if cfg.MirrorAuthor.Name == "" {
		cfg.MirrorAuthor.Name = user.Login
		if cfg.MirrorAuthor.Name == "" {
			cfg.MirrorAuthor.Name = "greenbridge"
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
