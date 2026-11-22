package app

import (
	"errors"

	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ident"
)

func checkGit(f *facts) result {
	if f.gitErr != nil {
		return fail("git", "git not found", "install git and put it on PATH")
	}
	return ok("git", f.git)
}

func checkConfig(f *facts) result {
	// Load's missing-file error ends in the init hint; the fix line carries it.
	switch {
	case errors.Is(f.cfgErr, config.ErrNoConfig):
		return fail("config", "no config at "+f.cfgPath, "run gbridge init")
	case f.cfgErr != nil:
		return fail("config", f.cfgErr.Error(), "run gbridge init")
	}
	return ok("config", "config loaded")
}

func checkSecret(f *facts) result {
	const fix = "run gbridge init, or gbridge init --secret <phrase> to reuse another machine's secret"
	switch {
	case errors.Is(f.secretErr, ident.ErrNoSecret):
		return fail("secret", "no secret at "+f.secretPath, fix)
	case f.secretErr != nil:
		return fail("secret", f.secretErr.Error(), fix)
	}
	return ok("secret", "secret present (same recovery phrase on every machine, or work double counts)")
}
