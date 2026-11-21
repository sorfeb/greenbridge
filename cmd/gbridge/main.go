// Command gbridge mirrors the timing of private or work commits onto your GitHub
// contribution graph.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"github.com/sorfeb/greenbridge/internal/app"
	"github.com/sorfeb/greenbridge/internal/brand"
	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ghx"
	"github.com/sorfeb/greenbridge/internal/ident"
)

// version is set at link time by GoReleaser. Builds from go install leave it
// empty and fall back to the module version in the build info.
var version string

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "dev"
}

// command is one gbridge subcommand. flags builds the flag set the command
// parses, bound to throwaway targets; completion scripts are generated from
// it, so a new flag can't drift out of them. nil means no flags.
type command struct {
	name, summary string
	flags         func() *flag.FlagSet
}

var commands = []command{
	{"init", "set up emails, source repos, secret, and the mirror repo", func() *flag.FlagSet { return new(initFlags).set() }},
	{"add", "add to the config: add email|repo|scan <value>...", nil},
	{"remove", "remove from the config: remove email|repo|scan <value>...", nil},
	{"config", "print the current setup", nil},
	{"plan", "preview what would be mirrored (writes nothing)", func() *flag.FlagSet { return new(planFlags).set() }},
	{"sync", "mirror new activity and push", func() *flag.FlagSet { return new(syncFlags).set("sync") }},
	{"backfill", "alias for sync; use --since to limit history", func() *flag.FlagSet { return new(syncFlags).set("backfill") }},
	{"doctor", "check every precondition for squares to appear", nil},
	{"secret", "print the recovery phrase for other machines", nil},
	{"completion", "print a shell completion script: " + strings.Join(shells, ", "), nil},
	{"version", "print the version", nil},
}

func usage() string {
	var b strings.Builder
	b.WriteString("gbridge: mirror private commit activity onto your GitHub graph\n\nUsage:\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "  gbridge %-10s  %s\n", c.name, c.summary)
	}
	b.WriteString("\nRun gbridge <command> -h for flags. Config lives in $GBRIDGE_HOME or the user config dir.\n")
	return b.String()
}

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

// pathList is a list of directories; completion offers directory names for it.
type pathList struct{ list }

// banner prints the bridge of the day, only when f is a terminal, so pipes
// and scripts see plain output.
func banner(f *os.File) {
	if term.IsTerminal(f.Fd()) {
		brand.Banner(f, time.Now(), lipgloss.NewRenderer(f))
	}
}

func main() {
	if len(os.Args) < 2 {
		banner(os.Stderr)
		fmt.Fprint(os.Stderr, usage())
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "init":
		err = runInit(args)
	case "add", "remove":
		err = runEdit(cmd, args)
	case "config":
		err = app.ShowConfig(os.Stdout)
	case "plan":
		err = runPlan(args)
	case "sync", "backfill":
		err = runSync(cmd, args)
	case "doctor":
		if app.Doctor(os.Stdout, ghx.NewCLI()) > 0 {
			os.Exit(1)
		}
	case "secret":
		err = runSecret()
	case "completion":
		err = runCompletion(os.Stdout, args)
	case "version", "--version":
		fmt.Println("gbridge", buildVersion())
	case "help", "-h", "--help":
		banner(os.Stdout)
		fmt.Print(usage())
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage())
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gbridge:", err)
		os.Exit(1)
	}
}

type initFlags struct {
	o           app.InitOptions
	emails      list
	repos, scan pathList
}

func (f *initFlags) set() *flag.FlagSet {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.Var(&f.emails, "email", "work email to match (repeatable)")
	fs.Var(&f.repos, "repo", "source repo path (repeatable)")
	fs.Var(&f.scan, "scan", "folder searched for repos on every plan and sync (repeatable)")
	fs.BoolVar(&f.o.IncludeGitHub, "include-github", false, "include repos with a github.com remote (double counts)")
	fs.StringVar(&f.o.MirrorName, "mirror", "", "mirror repo name (default greenbridge-activity)")
	fs.BoolVar(&f.o.LocalOnly, "local-only", false, "keep the mirror local; create nothing on GitHub")
	fs.BoolVar(&f.o.Public, "public", false, "public mirror with date-only timestamps")
	fs.StringVar(&f.o.Secret, "secret", "", "recovery phrase from another machine")
	fs.StringVar(&f.o.AuthorName, "author-name", "", "mirror commit author name (default GitHub login)")
	fs.StringVar(&f.o.AuthorEmail, "author-email", "", "mirror commit author email (default GitHub noreply)")
	fs.BoolVar(&f.o.Yes, "yes", false, "accept defaults without prompting")
	fs.BoolVar(&f.o.Force, "force", false, "replace existing config or secret")
	return fs
}

func runInit(args []string) error {
	var f initFlags
	_ = f.set().Parse(args)
	f.o.Emails, f.o.Repos, f.o.Scan = f.emails, f.repos.list, f.scan.list
	banner(os.Stdout)
	return app.Init(f.o, ghx.NewCLI(), os.Stdin, os.Stdout)
}

// runEdit runs gbridge add or remove: the first argument is the kind, the
// rest are values.
func runEdit(cmd string, args []string) error {
	kind := ""
	if len(args) > 0 {
		kind, args = args[0], args[1:]
	}
	if cmd == "add" {
		return app.Add(kind, args, os.Stdout)
	}
	return app.Remove(kind, args, os.Stdout)
}

func load() (*config.Config, []byte, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	sp, err := config.SecretPath()
	if err != nil {
		return nil, nil, err
	}
	secret, err := ident.Load(sp)
	return cfg, secret, err
}

func parseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return t, errors.New("--since wants YYYY-MM-DD")
	}
	return t, nil
}

type planFlags struct {
	since   string
	weeks   int
	offline bool
}

func (f *planFlags) set() *flag.FlagSet {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	fs.StringVar(&f.since, "since", "", "only commits on or after YYYY-MM-DD")
	fs.IntVar(&f.weeks, "weeks", 53, "weeks shown in the graph")
	fs.BoolVar(&f.offline, "offline", false, "skip fetching the mirror")
	return fs
}

func runPlan(args []string) error {
	var f planFlags
	_ = f.set().Parse(args)
	cfg, secret, err := load()
	if err != nil {
		return err
	}
	t, err := parseSince(f.since)
	if err != nil {
		return err
	}
	p, err := app.ComputePlan(cfg, secret, app.PlanOptions{Since: t, Offline: f.offline})
	if err != nil {
		return err
	}
	p.Report(os.Stdout, time.Now(), f.weeks, brand.Paint(lipgloss.NewRenderer(os.Stdout)))
	return nil
}

type syncFlags struct {
	since       string
	dry, noPush bool
}

func (f *syncFlags) set(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.StringVar(&f.since, "since", "", "only commits on or after YYYY-MM-DD")
	fs.BoolVar(&f.dry, "dry-run", false, "compute only, write nothing")
	fs.BoolVar(&f.noPush, "no-push", false, "write the local mirror, skip push")
	return fs
}

func runSync(name string, args []string) error {
	var f syncFlags
	_ = f.set(name).Parse(args)
	cfg, secret, err := load()
	if err != nil {
		return err
	}
	t, err := parseSince(f.since)
	if err != nil {
		return err
	}
	res, err := app.Sync(cfg, secret, app.SyncOptions{Since: t, DryRun: f.dry, NoPush: f.noPush}, os.Stderr)
	if err != nil {
		return err
	}
	app.WriteNewRepos(os.Stdout, res.NewRepos)
	verb := "added"
	if f.dry {
		verb = "would add"
	}
	fmt.Printf("%d desired, %d already mirrored, %s %d", res.Desired, res.Ledger, verb, res.Added)
	if res.Pushed {
		fmt.Printf(", pushed to %s", cfg.Mirror.Repo)
	}
	fmt.Println()
	return nil
}

func runSecret() error {
	sp, err := config.SecretPath()
	if err != nil {
		return err
	}
	s, err := ident.Load(sp)
	if err != nil {
		return err
	}
	fmt.Println(ident.Encode(s))
	return nil
}
