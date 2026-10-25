package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/template"
)

// shells lists the shells gbridge completion supports, one template each.
var shells = []string{"bash", "zsh", "fish", "powershell"}

//go:embed shell/*.tmpl
var shellTemplates embed.FS

// compFlag is a flag as the completion scripts see it.
type compFlag struct {
	Name, Usage string
	Value       bool // takes a value; false for booleans
	Dir         bool // the value is a directory
	Repeat      bool // may be given more than once
}

type compCommand struct {
	Name, Summary string
	Flags         []compFlag
}

type compData struct {
	Commands []compCommand
	Names    string // every command, space separated
	Shells   string
}

func completionData() compData {
	d := compData{Shells: strings.Join(shells, " ")}
	var names []string
	for _, c := range commands {
		cc := compCommand{Name: c.name, Summary: c.summary}
		if c.flags != nil {
			c.flags().VisitAll(func(f *flag.Flag) {
				b, isBool := f.Value.(interface{ IsBoolFlag() bool })
				_, dir := f.Value.(*pathList)
				_, l := f.Value.(*list)
				cc.Flags = append(cc.Flags, compFlag{
					Name: f.Name, Usage: f.Usage,
					Value: !isBool || !b.IsBoolFlag(), Dir: dir, Repeat: dir || l,
				})
			})
		}
		d.Commands = append(d.Commands, cc)
		names = append(names, c.name)
	}
	d.Names = strings.Join(names, " ")
	return d
}

// Each shell quotes differently inside its single-quoted strings.
var quote = template.FuncMap{
	"sq": strings.NewReplacer(`'`, `'\''`).Replace,                       // POSIX shells
	"zq": strings.NewReplacer(`'`, `'\''`, `[`, `\[`, `]`, `\]`).Replace, // zsh _arguments [description]
	"fq": strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace,              // fish
	"pq": strings.NewReplacer(`'`, `''`).Replace,                         // PowerShell
	"pattern": func(fs []compFlag, dir bool) string { // bash case pattern for value flags
		var p []string
		for _, f := range fs {
			if f.Value && f.Dir == dir {
				p = append(p, "--"+f.Name, "-"+f.Name)
			}
		}
		return strings.Join(p, "|")
	},
	"words": func(fs []compFlag) string {
		var w []string
		for _, f := range fs {
			w = append(w, "--"+f.Name)
		}
		return strings.Join(w, " ")
	},
}

func runCompletion(w io.Writer, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: gbridge completion <shell>, one of %s", strings.Join(shells, ", "))
	}
	known := false
	for _, s := range shells {
		known = known || s == args[0]
	}
	if !known {
		return fmt.Errorf("no completion for %q; supported: %s", args[0], strings.Join(shells, ", "))
	}
	t, err := template.New(args[0]+".tmpl").Funcs(quote).ParseFS(shellTemplates, "shell/"+args[0]+".tmpl")
	if err != nil {
		return err
	}
	return t.Execute(w, completionData())
}
