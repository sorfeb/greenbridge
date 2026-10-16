// Command gb mirrors the timing of private or work commits onto your GitHub
// contribution graph.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"github.com/sorfeb/greenbridge/internal/app"
	"github.com/sorfeb/greenbridge/internal/brand"
	"github.com/sorfeb/greenbridge/internal/config"
	"github.com/sorfeb/greenbridge/internal/ident"
)

var version = "dev"

const usage = `gb: mirror private commit activity onto your GitHub graph

Usage:
  gb init      set up emails, source repos, secret, and the mirror repo
  gb plan      preview what would be mirrored (writes nothing)
  gb sync      mirror new activity and push
  gb backfill  alias for sync; use --since to limit history
  gb doctor    check every precondition for squares to appear
  gb secret    print the recovery phrase for other machines
  gb version

Run gb <command> -h for flags. Config lives in $GB_HOME or the user config dir.
`

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

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
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "init":
		err = runInit(args)
	case "plan":
		err = runPlan(args)
	case "sync", "backfill":
		err = runSync(cmd, args)
	case "doctor":
		if app.Doctor(os.Stdout) > 0 {
			os.Exit(1)
		}
	case "secret":
		err = runSecret()
	case "version", "--version":
		fmt.Println("gb", version)
	case "help", "-h", "--help":
		banner(os.Stdout)
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gb:", err)
		os.Exit(1)
	}
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	var o app.InitOptions
	var emails, repos, scan list
	fs.Var(&emails, "email", "work email to match (repeatable)")
	fs.Var(&repos, "repo", "source repo path (repeatable)")
	fs.Var(&scan, "scan", "folder to search for repos (repeatable)")
	fs.BoolVar(&o.IncludeGitHub, "include-github", false, "include repos with a github.com remote (double counts)")
	fs.StringVar(&o.MirrorName, "mirror", "", "mirror repo name (default greenbridge-activity)")
	fs.BoolVar(&o.LocalOnly, "local-only", false, "keep the mirror local; create nothing on GitHub")
	fs.BoolVar(&o.Public, "public", false, "public mirror with date-only timestamps")
	fs.StringVar(&o.Secret, "secret", "", "recovery phrase from another machine")
	fs.StringVar(&o.AuthorName, "author-name", "", "mirror commit author name (default GitHub login)")
	fs.StringVar(&o.AuthorEmail, "author-email", "", "mirror commit author email (default GitHub noreply)")
	fs.BoolVar(&o.Yes, "yes", false, "accept defaults without prompting")
	fs.BoolVar(&o.Force, "force", false, "replace existing config or secret")
	_ = fs.Parse(args)
	o.Emails, o.Repos, o.Scan = emails, repos, scan
	banner(os.Stdout)
	return app.Init(o, os.Stdin, os.Stdout)
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

func runPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	since := fs.String("since", "", "only commits on or after YYYY-MM-DD")
	weeks := fs.Int("weeks", 53, "weeks shown in the graph")
	offline := fs.Bool("offline", false, "skip fetching the mirror")
	_ = fs.Parse(args)
	cfg, secret, err := load()
	if err != nil {
		return err
	}
	t, err := parseSince(*since)
	if err != nil {
		return err
	}
	d, err := app.Collect(cfg, secret, t)
	if err != nil {
		return err
	}
	for _, r := range d.Repos {
		if r.Skipped != "" {
			fmt.Printf("  skip %s: %s\n", r.Path, r.Skipped)
		} else {
			fmt.Printf("  %5d  %s\n", r.Matched, r.Path)
		}
	}
	fmt.Printf("\n%d commits matched, %d after dedup", d.Matched, len(d.Entries))
	if len(d.Entries) > 0 {
		fmt.Printf(", %s to %s", app.DayKey(d.Entries[0].Time), app.DayKey(d.Entries[len(d.Entries)-1].Time))
	}
	fmt.Println()

	m := app.MirrorOf(cfg)
	if *offline {
		m.Remote = ""
	}
	if err := m.Ensure(); err != nil {
		return err
	}
	tip, err := m.Fetch()
	if err != nil {
		fmt.Printf("mirror fetch failed (%v); showing all as new\n", err)
	}
	ledger, err := m.Ledger(tip)
	if err != nil {
		return err
	}
	fmt.Printf("already in mirror: %d, would add: %d\n\n", len(ledger), len(app.Diff(d.Entries, ledger)))
	app.YearTable(os.Stdout, d.Entries)
	fmt.Println()
	app.Graph(os.Stdout, d.Entries, *weeks, time.Now(), brand.Paint(lipgloss.NewRenderer(os.Stdout)))
	return nil
}

func runSync(name string, args []string) error {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	since := fs.String("since", "", "only commits on or after YYYY-MM-DD")
	dry := fs.Bool("dry-run", false, "compute only, write nothing")
	noPush := fs.Bool("no-push", false, "write the local mirror, skip push")
	_ = fs.Parse(args)
	cfg, secret, err := load()
	if err != nil {
		return err
	}
	t, err := parseSince(*since)
	if err != nil {
		return err
	}
	res, err := app.Sync(cfg, secret, app.SyncOptions{Since: t, DryRun: *dry, NoPush: *noPush}, os.Stderr)
	if err != nil {
		return err
	}
	verb := "added"
	if *dry {
		verb = "would add"
	}
	fmt.Printf("%d desired, %d already mirrored, %s %d", res.Desired, res.Existing, verb, res.Added)
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
