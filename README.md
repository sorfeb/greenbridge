<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/hero-dark.svg">
    <img src=".github/assets/hero-light.svg" alt="greenbridge: the Golden Gate Bridge drawn in contribution-graph squares" width="640">
  </picture>
</h1>

<p align="center">Mirror commits GitHub can't see onto your GitHub contribution graph, without copying any code.</p>

<p align="center">
  <a href="https://github.com/sorfeb/greenbridge/actions/workflows/ci.yml"><img src="https://github.com/sorfeb/greenbridge/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/sorfeb/greenbridge/releases/latest"><img src="https://img.shields.io/github/v/release/sorfeb/greenbridge?sort=semver" alt="Latest release"></a>
  <a href="https://pkg.go.dev/github.com/sorfeb/greenbridge"><img src="https://pkg.go.dev/badge/github.com/sorfeb/greenbridge.svg" alt="Go reference"></a>
  <a href="https://goreportcard.com/report/github.com/sorfeb/greenbridge"><img src="https://goreportcard.com/badge/github.com/sorfeb/greenbridge" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/sorfeb/greenbridge" alt="MIT license"></a>
</p>

Work on an employer GitLab, a self-hosted Forgejo, or an on-prem server never reaches your GitHub graph. `gbridge` reads commit metadata from your local clones and writes one empty, backdated commit per real commit to a private mirror repo you own. GitHub then counts those commits like any other.

## Table of contents

- [Security](#security)
- [Background](#background)
- [Install](#install)
- [Usage](#usage)
- [The bridge of the day](#the-bridge-of-the-day)
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

The secret lives in your config directory. Back up the phrase from `gbridge secret`: losing it means the next sync can't recognise what is already mirrored, and double counts.

## Background

GitHub counts a commit when it is on the default branch of a non-fork repo and its author email belongs to your account ([reference](https://docs.github.com/en/account-and-profile/reference/profile-contributions-reference)). Commits are placed on the day of their author date, in the commit's own time zone. An empty commit counts the same as any other.

`gbridge` uses exactly that: it never fabricates activity, only mirrors commits you wrote, one to one, at the time you wrote them. Tools that paint patterns or pad streaks break GitHub's [acceptable use policy](https://docs.github.com/en/site-policy/acceptable-use-policies/github-acceptable-use-policies#4-spam-and-inauthentic-activity-on-github) on inauthentic activity. `gbridge` is designed to stay on the right side of it.

Check your employment contract before mirroring employer work. Only timestamps leave your machine, but some employers treat even those as internal.

## Install

Requires `git` and the [GitHub CLI](https://cli.github.com) logged in (`gh auth login`).

Download the archive for your platform from [Releases](https://github.com/sorfeb/greenbridge/releases) and put `gbridge` on your `PATH`. Or, with Go 1.27+:

```sh
go install github.com/sorfeb/greenbridge/cmd/gbridge@latest
```

While this repo is private, set `GOPRIVATE=github.com/sorfeb/*` first, or run `go install ./cmd/gbridge` from a clone.

The command is `gbridge`, not `gb`: oh-my-zsh and prezto alias `gb` to `git branch`.

### Verify a download

Each release ships `checksums.txt`, an SBOM per archive, and a GitHub build provenance attestation. To confirm an archive was built by this repo's release workflow and not uploaded by hand:

```sh
gh attestation verify gbridge_<version>_linux_amd64.tar.gz --repo sorfeb/greenbridge
```

## Usage

```sh
gbridge init --email you@work.com --scan ~/work   # create a private greenbridge-activity repo
gbridge plan                                      # preview, writes nothing
gbridge sync                                      # mirror and push; rerun any time
gbridge doctor                                    # explain missing squares
```

### CLI

| Command | Does | Useful flags |
|---|---|---|
| `gbridge init` | Sets up emails, source repos, the secret, and the mirror repo | `--email`, `--repo`, `--scan` (repeatable), `--mirror`, `--public`, `--local-only`, `--secret` |
| `gbridge plan` | Previews counts per year and a terminal graph | `--since YYYY-MM-DD`, `--offline` |
| `gbridge sync` | Adds missing commits to the mirror and pushes | `--since`, `--dry-run`, `--no-push` |
| `gbridge backfill` | Alias for `sync`, for recovering past work | `--since` |
| `gbridge doctor` | Checks every condition GitHub needs to show a square | |
| `gbridge secret` | Prints the recovery phrase for a second machine | |
| `gbridge completion` | Prints a tab-completion script | `bash`, `zsh`, `fish`, `powershell` |

- **Past work** is recovered on the first sync, as far back as your clones go.
- **Reruns are safe.** `gbridge sync` reads already-synced IDs back from the mirror's log. There is no state file.
- **Several machines** can sync into one mirror. Run `gbridge secret` on the first, then `gbridge init --secret <phrase>` on the second. A different secret produces different IDs and double counts.
- **Private mirror:** turn on **Include private contributions on my profile** at <https://github.com/settings/profile>. `gbridge doctor` reports this setting as unverified, since no GitHub API exposes it.
- `GBRIDGE_HOME` overrides the config directory.

### Tab completion

Completes commands, each command's flags, and directories after `--repo` and `--scan`. Add one line for your shell:

| Shell | Add |
|---|---|
| bash | `source <(gbridge completion bash)` to `~/.bashrc` |
| zsh | `source <(gbridge completion zsh)` to `~/.zshrc`, after `compinit` |
| fish | Run once: `gbridge completion fish > ~/.config/fish/completions/gbridge.fish` |
| PowerShell | `gbridge completion powershell \| Out-String \| Invoke-Expression` to `$PROFILE` |

Release archives also include the four scripts under `completions/`.

## The bridge of the day

`gbridge help`, `gbridge init`, and a bare `gbridge` open with a famous bridge in contribution-graph squares. The bridge and its greens change once a day and match for everyone on that day. Pipes and scripts get plain text.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/assets/bridges-dark.svg">
  <img src=".github/assets/bridges-light.svg" alt="The ten bridges gbridge rotates through: Golden Gate, Brooklyn, Tower Bridge, Sydney Harbour, Pont du Gard, Ponte Vecchio, Rialto, Forth Bridge, Millau Viaduct, and Charles Bridge" width="528">
</picture>

## What counts

`gbridge` mirrors a commit when all of these hold:

- Its author email is one you gave `gbridge init`.
- It is reachable from any local branch, remote-tracking branch, or tag in a source clone.
- Its repo has no github.com remote, since GitHub already counts those. Override at init with `--include-github`.

Duplicates collapse: a rebased, cherry-picked, or amended commit keeps its author email and date, so it maps to the same ID and counts once.

### How this differs from GitHub

| | GitHub, for github.com repos | `gbridge` today |
|---|---|---|
| Branches | Default branch only; feature work appears once merged | Every branch, merged or not |
| Merge commits | Counted | Counted |
| Date used | Author date, commit's own time zone | Same |
| Pull requests, issues, reviews | One contribution each | Not mirrored; git history doesn't record them |

### Why your forge shows more

Forgejo and Gitea heatmaps count feed actions, not commits: every push, opened or merged pull request, comment, and branch change adds one. `gbridge` sees only commits, so its day totals match your git history and sit below the forge's heatmap. That gap is expected.

## Keeping the graph current

There is no automatic trigger yet. Run `gbridge sync` after you push.

A local sync costs about 0.5 s. Network round trips to GitHub (check the mirror, fetch, push) add 3-6 s. The planned pre-push hook will only queue a request and start a detached `gbridge sync`, so it adds milliseconds to your push and never blocks or fails it.

## Limitations

- **Clones only.** Commits that never reached a local clone are invisible. Run `git fetch --all` before syncing.
- **Removal is manual.** Sync only adds. Removing mirrored commits means rebuilding the mirror and force-pushing it.
- **One mirror per config.** Splitting activity per employer isn't supported yet.

## Contributing

Questions and bug reports go to [GitHub issues](https://github.com/sorfeb/greenbridge/issues); security reports go through [SECURITY.md](SECURITY.md). See [CONTRIBUTING.md](CONTRIBUTING.md) for setup and conventions. In short, with Go 1.27 and nothing else installed:

```sh
go tool -modfile tools.mod task check   # what CI runs
go tool -modfile tools.mod task fix     # auto-fix lint and formatting
```

## License

[MIT](LICENSE)
