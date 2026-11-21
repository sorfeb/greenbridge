package gitx

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"os/exec"
	"strings"
	"time"
)

// Lines runs git in dir and yields its output one line at a time, without
// the line ending, as git writes it: memory stays flat however much git
// prints. A failing git ends the sequence with one error carrying git's
// stderr. Breaking out of the loop stops git; either way git has exited and
// been waited on when the loop ends.
func Lines(dir string, args ...string) iter.Seq2[string, error] {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	return func(yield func(string, error) bool) {
		fail := func(err error, stderr string) {
			yield("", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr)))
		}
		cmd := exec.Command("git", args...)
		// A child git spawns may hold stderr open after git is killed.
		cmd.WaitDelay = time.Second
		var errb bytes.Buffer
		cmd.Stderr = &errb
		out, err := cmd.StdoutPipe()
		if err != nil {
			fail(err, "")
			return
		}
		if err := cmd.Start(); err != nil {
			fail(err, "")
			return
		}
		waited := false
		defer func() {
			// The loop broke, or yield panicked: git may be blocked writing
			// to a full pipe nobody reads.
			if !waited {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}()
		r := bufio.NewReader(out)
		for {
			line, rerr := r.ReadString('\n')
			if line != "" && !yield(strings.TrimRight(line, "\r\n"), nil) {
				return
			}
			if errors.Is(rerr, io.EOF) {
				break
			}
			if rerr != nil {
				fail(rerr, "")
				return
			}
		}
		waited = true
		if err := cmd.Wait(); err != nil {
			fail(err, errb.String())
		}
	}
}
