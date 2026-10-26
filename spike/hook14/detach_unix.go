//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach puts the child in a new session: no controlling terminal, no SIGHUP
// when the shell that ran `git push` exits.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func consoleState() string { return "n/a" }
