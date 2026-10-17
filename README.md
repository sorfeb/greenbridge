# greenbridge

Mirror commits GitHub can't see onto your GitHub contribution graph, without copying any code.

Work on an employer GitLab, a self-hosted Forgejo, or an on-prem server never reaches your GitHub graph. `gb` reads commit metadata from your local clones and writes one empty, backdated commit per real commit to a private mirror repo you own. GitHub then counts those commits like any other.

## Table of contents

- [Security](#security)
- [Background](#background)
- [Install](#install)
- [Usage](#usage)
- [What counts](#what-counts)
- [Keeping the graph current](#keeping-the-graph-current)
- [Limitations](#limitations)
- [Contributing](#contributing)
- [License](#license)

## Security

Per source commit, only two things leave your machine:

- a timestamp, exact for a private mirror and date only for a public one
- an opaque 12-character ID, `HMAC(secret, email NUL author_unix)`

Code, diffs, file names, commit messages, repo and branch names, commit hashes, and your work email stay local. Without the secret, nobody can match a mirror ID to a source commit, including people with access to the source repo.

The secret lives in your config directory. Back up the phrase from `gb secret`: losing it means the next sync can't recognise what is already mirrored, and double counts.

## Background

GitHub counts a commit when it is on the default branch of a non-fork repo and its author email belongs to your account ([reference](https://docs.github.com/en/account-and-profile/reference/profile-contributions-reference)). Commits are placed on the day of their author date, in the commit's own time zone. An empty commit counts the same as any other.

`gb` uses exactly that: it never fabricates activity, only mirrors commits you wrote, one to one, at the time you wrote them. Tools that paint patterns or pad streaks break GitHub's [acceptable use policy](https://docs.github.com/en/site-policy/acceptable-use-policies/github-acceptable-use-policies#4-spam-and-inauthentic-activity-on-github) on inauthentic activity. `gb` is designed to stay on the right side of it.

Check your employment contract before mirroring employer work. Only timestamps leave your machine, but some employers treat even those as internal.

## Install

Requires Go 1.27+, `git`, and the [GitHub CLI](https://cli.github.com) logged in (`gh auth login`).

```sh
go install github.com/sorfeb/greenbridge/cmd/gb@latest
```

While this repo is private, set `GOPRIVATE=github.com/sorfeb/*` first, or run `go install ./cmd/gb` from a clone.

## Usage

```sh
gb init --email you@work.com --scan ~/work   # create a private greenbridge-activity repo
gb plan                                      # preview, writes nothing
gb sync                                      # mirror and push; rerun any time
gb doctor                                    # explain missing squares
```

### CLI

| Command | Does | Useful flags |
|---|---|---|
| `gb init` | Sets up emails, source repos, the secret, and the mirror repo | `--email`, `--repo`, `--scan` (repeatable), `--mirror`, `--public`, `--local-only`, `--secret` |
| `gb plan` | Previews counts per year and a terminal graph | `--since YYYY-MM-DD`, `--offline` |
| `gb sync` | Adds missing commits to the mirror and pushes | `--since`, `--dry-run`, `--no-push` |
| `gb backfill` | Alias for `sync`, for recovering past work | `--since` |
| `gb doctor` | Checks every condition GitHub needs to show a square | |
| `gb secret` | Prints the recovery phrase for a second machine | |

- **Past work** is recovered on the first sync, as far back as your clones go.
- **Reruns are safe.** `gb sync` reads already-synced IDs back from the mirror's log. There is no state file.
- **Several machines** can sync into one mirror. Run `gb secret` on the first, then `gb init --secret <phrase>` on the second. A different secret produces different IDs and double counts.
- **Private mirror:** turn on **Include private contributions on my profile** at <https://github.com/settings/profile>. `gb doctor` reports this setting as unverified, since no GitHub API exposes it.
- `GB_HOME` overrides the config directory.

## What counts

`gb` mirrors a commit when all of these hold:

- Its author email is one you gave `gb init`.
- It is reachable from any local branch, remote-tracking branch, or tag in a source clone.
- Its repo has no github.com remote, since GitHub already counts those. Override at init with `--include-github`.

Duplicates collapse: a rebased, cherry-picked, or amended commit keeps its author email and date, so it maps to the same ID and counts once.

### How this differs from GitHub

| | GitHub, for github.com repos | `gb` today |
|---|---|---|
| Branches | Default branch only; feature work appears once merged | Every branch, merged or not |
| Merge commits | Counted | Counted |
| Date used | Author date, commit's own time zone | Same |
| Pull requests, issues, reviews | One contribution each | Not mirrored; git history doesn't record them |

### Why your forge shows more

Forgejo and Gitea heatmaps count feed actions, not commits: every push, opened or merged pull request, comment, and branch change adds one. `gb` sees only commits, so its day totals match your git history and sit below the forge's heatmap. That gap is expected.

## Keeping the graph current

There is no automatic trigger yet. Run `gb sync` after you push.

A local sync costs about 0.5 s. Network round trips to GitHub (check the mirror, fetch, push) add 3-6 s. The planned pre-push hook will only queue a request and start a detached `gb sync`, so it adds milliseconds to your push and never blocks or fails it.

## Limitations

- **Clones only.** Commits that never reached a local clone are invisible. Run `git fetch --all` before syncing.
- **Removal is manual.** Sync only adds. Removing mirrored commits means rebuilding the mirror and force-pushing it.
- **One mirror per config.** Splitting activity per employer isn't supported yet.

## Contributing

Planning lives on the maintainer's Linear board (project greenbridge). Questions and bug reports go to [GitHub issues](https://github.com/sorfeb/greenbridge/issues).

Before sending a pull request, run the same checks as CI. Needs Go 1.27; dev tools (golangci-lint, Task, gotestsum, govulncheck) are pinned in `tools.mod`, so there is nothing else to install.

```sh
go tool -modfile=tools.mod task check   # what CI runs
go tool -modfile=tools.mod task fix     # auto-fix lint and formatting
```

## License

No license chosen yet. Until one is added, all rights are reserved by the author.
