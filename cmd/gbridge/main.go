// Command gbridge mirrors the timing of private or work commits onto your GitHub
// contribution graph.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
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
	// words are the values the first argument takes, offered by completion.
	words []string
	// usage is the -h text of a command without flags: one line per form,
	// then an optional note. Empty means the name and summary.
	usage []usageLine
	note  string
}

// usageLine is one form of a command in its -h text.
type usageLine struct{ args, what string }

// editKind is one thing add and remove change in the config.
type editKind struct{ name, arg, add, remove string }

var editKinds = []editKind{
	{"email", "<address>...", "add work emails to match", "stop matching work emails"},
	{"repo", "<path>...", "add source repos", "stop reading source repos"},
	{"scan", "<folder>...", "add scan folders, searched for source repos on every plan and sync", "stop searching scan folders"},
}

func editWords() []string {
	var w []string
	for _, k := range editKinds {
		w = append(w, k.name)
	}
	return w
}

func editUsage(remove bool) []usageLine {
	var u []usageLine
	for _, k := range editKinds {
		what := k.add
		if remove {
			what = k.remove
		}
		u = append(u, usageLine{k.name + " " + k.arg, what})
	}
	return u
}

var commands = []command{
	{name: "init", summary: "set up emails, source repos, secret, and the mirror repo", flags: func() *flag.FlagSet { return new(initFlags).set() }},
	{
		name: "add", summary: "add to the config: add email|repo|scan <value>...",
		words: editWords(), usage: editUsage(false),
		note: "Changes only the config: the secret and the mirror repo stay as they are.",
	},
	{
		name: "remove", summary: "remove from the config: remove email|repo|scan <value>...",
		words: editWords(), usage: editUsage(true),
		note: "Commits already mirrored stay in the mirror repo.",
	},
	{name: "config", summary: "print the current setup"},
	{name: "plan", summary: "preview what would be mirrored (writes nothing)", flags: func() *flag.FlagSet { return new(planFlags).set() }},
	{name: "sync", summary: "mirror new activity and push", flags: func() *flag.FlagSet { return new(syncFlags).set("sync") }},
	{name: "backfill", summary: "alias for sync; use --since to limit history", flags: func() *flag.FlagSet { return new(syncFlags).set("backfill") }},
	{name: "doctor", summary: "check every precondition for squares to appear"},
	{name: "secret", summary: "print the recovery phrase for other machines"},
	{
		name: "completion", summary: "print a shell completion script: " + strings.Join(shells, ", "),
		words: shells, usage: []usageLine{{"<shell>", "print the completion script for " + strings.Join(shells, ", ")}},
	},
	{name: "version", summary: "print the version"},
}

// printHelp writes the -h text of a command without flags and reports
// whether args asked for it. Commands with flags get theirs from the flag
// package. No email, path, or folder is spelled -h, so help wins anywhere in
// args.
func printHelp(w io.Writer, name string, args []string) bool {
	i := slices.IndexFunc(commands, func(c command) bool { return c.name == name })
	if i < 0 || commands[i].flags != nil || !slices.ContainsFunc(args, func(a string) bool {
		return a == "-h" || a == "-help" || a == "--help"
	}) {
		return false
	}
	c := commands[i]
	usage := c.usage
	if usage == nil {
		usage = []usageLine{{"", c.summary}}
	}
	width := 0
	for _, u := range usage {
		width = max(width, len(strings.TrimSpace(c.name+" "+u.args)))
	}
	fmt.Fprintln(w, "Usage:")
	for _, u := range usage {
		fmt.Fprintf(w, "  gbridge %-*s  %s\n", width, strings.TrimSpace(c.name+" "+u.args), u.what)
	}
	if c.note != "" {
		fmt.Fprintf(w, "\n%s\n", c.note)
	}
	return true
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
	cmd, args := os.Args[1], os.Args[2:]
	if printHelp(os.Stdout, cmd, args) {
		return
	}
	var err error
	switch cmd {
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
	fetch   bool
}

func (f *planFlags) set() *flag.FlagSet {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	fs.StringVar(&f.since, "since", "", "only commits on or after YYYY-MM-DD")
	fs.IntVar(&f.weeks, "weeks", 53, "weeks shown in the graph")
	fs.BoolVar(&f.offline, "offline", false, "skip fetching the mirror")
	fs.BoolVar(&f.fetch, "fetch", false, "git fetch every source repo first")
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
	p, err := app.ComputePlan(cfg, secret, app.PlanOptions{Since: t, Offline: f.offline, Fetch: f.fetch})
	if err != nil {
		return err
	}
	p.Report(os.Stdout, time.Now(), f.weeks, brand.Paint(lipgloss.NewRenderer(os.Stdout)))
	return nil
}

type syncFlags struct {
	since              string
	dry, noPush, fetch bool
}

func (f *syncFlags) set(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.StringVar(&f.since, "since", "", "only commits on or after YYYY-MM-DD")
	fs.BoolVar(&f.dry, "dry-run", false, "compute only, write nothing")
	fs.BoolVar(&f.noPush, "no-push", false, "write the local mirror, skip push")
	fs.BoolVar(&f.fetch, "fetch", false, "git fetch every source repo first")
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
	res, err := app.Sync(cfg, secret, app.SyncOptions{Since: t, DryRun: f.dry, NoPush: f.noPush, Fetch: f.fetch}, os.Stderr)
	if err != nil {
		return err
	}
	app.WriteFetchFailed(os.Stderr, res.FetchFailed)
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
	app.WriteOrphans(os.Stdout, res.Orphans)
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
