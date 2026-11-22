package app

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// prompter asks questions on a reader and a writer. With yes set, every
// question takes its default without asking.
type prompter struct {
	rd  *bufio.Reader
	out io.Writer
	yes bool
}

func newPrompter(in io.Reader, out io.Writer, yes bool) *prompter {
	return &prompter{rd: bufio.NewReader(in), out: out, yes: yes}
}

func (p *prompter) ask(q, def string) string {
	if p.yes {
		return def
	}
	fmt.Fprintf(p.out, "%s [%s]: ", q, def)
	line, _ := p.rd.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return def
	}
	return line
}

func (p *prompter) confirm(q string) bool {
	a := strings.ToLower(p.ask(q+" (y/n)", "y"))
	return a == "y" || a == "yes"
}

func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
