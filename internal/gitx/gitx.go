// Package gitx runs the git binary. greenbridge shells out rather than
// linking a git library so behavior matches the user's own git exactly.
package gitx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Run executes git in dir (empty means current directory) with extra env
// entries and optional stdin. Output has trailing newlines trimmed.
func Run(dir string, env []string, stdin io.Reader, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = stdin
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimRight(out.String(), "\r\n"), nil
}

// Git is Run without env or stdin.
func Git(dir string, args ...string) (string, error) {
	return Run(dir, nil, nil, args...)
}
