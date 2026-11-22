package app

import (
	"fmt"
	"io"
	"os"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/ident"
)

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
	return MirrorClone(d.cfg).Ensure()
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
