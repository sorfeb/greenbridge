package main

import (
	"errors"
	"flag"
	"os"

	"github.com/sorfeb/greenbridge/internal/app"
	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/hook"
)

// backgroundFiles are the background sync files in the greenbridge home.
func backgroundFiles() (hook.Files, error) {
	home, err := config.Dir()
	return hook.FilesIn(home), err
}

// startBackground starts this binary with args as a background sync.
func startBackground(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	files, err := backgroundFiles()
	if err != nil {
		return err
	}
	return hook.Detach(exe, args, files)
}

func runHook(args []string) error {
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "install":
		fs := flag.NewFlagSet("hook install", flag.ExitOnError)
		perRepo := fs.Bool("per-repo", false, "write a pre-push hook into each source repo instead (for Git before 2.54)")
		_ = fs.Parse(args)
		exe, err := hook.Executable()
		if err != nil {
			return err
		}
		return app.HookInstall(exe, *perRepo, os.Stdout)
	case "uninstall":
		return app.HookUninstall(os.Stdout)
	case "status":
		return app.HookStatus(os.Stdout)
	case hook.Event:
		prePush()
		return nil
	}
	return errors.New("usage: gbridge hook install|uninstall|status")
}

// prePush is what the push hook runs. It must never fail the push, so it
// reports nothing to git: its output goes to /dev/null and its exit code is
// 0, even on a panic. Errors land in the background sync log instead.
func prePush() {
	defer func() { _ = recover() }()
	dir, err := os.Getwd()
	if err == nil {
		_, err = app.PrePush(dir, func() error { return startBackground([]string{"sync"}) })
	}
	if err != nil {
		if files, ferr := backgroundFiles(); ferr == nil {
			hook.LogFailure(files, "push hook: "+err.Error())
		}
	}
}
