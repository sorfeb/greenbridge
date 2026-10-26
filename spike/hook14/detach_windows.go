package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// detach gives the child its own process group, so Ctrl+C in the terminal
// does not reach the sync. Nil stdio becomes NUL, and Go passes only the
// listed handles, so no pipe leaks through inheritance.
//
// GBSPIKE_FLAGS=detached uses DETACHED_PROCESS (no console at all); the
// default uses CREATE_NO_WINDOW (a hidden console that git children inherit).
func detach(cmd *exec.Cmd) {
	flags := uint32(createNewProcessGroup | createNoWindow)
	if os.Getenv("GBSPIKE_FLAGS") == "detached" {
		flags = createNewProcessGroup | detachedProcess
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
}

var (
	getConsoleWindow = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	isWindowVisible  = syscall.NewLazyDLL("user32.dll").NewProc("IsWindowVisible")
)

// consoleState reports whether this process has a console window and whether
// it is visible. A visible one in a grandchild of the hook is a window that
// flashes up on every push.
func consoleState() string {
	h, _, _ := getConsoleWindow.Call()
	if h == 0 {
		return "no console window"
	}
	v, _, _ := isWindowVisible.Call(h)
	return fmt.Sprintf("console window, visible=%v", v != 0)
}
