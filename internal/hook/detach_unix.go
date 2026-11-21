//go:build !windows

package hook

import (
	"os/exec"
	"syscall"
)

// detach puts the child in a new session: no controlling terminal, and no
// SIGHUP when the shell that ran git push exits.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
