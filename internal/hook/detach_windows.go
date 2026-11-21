package hook

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// detach gives the child its own process group, so Ctrl+C in the terminal
// doesn't reach it, and a hidden console that its git children inherit.
// DETACHED_PROCESS would leave it no console at all, and every git it
// starts would flash a new window (issue #14).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | createNoWindow}
}
