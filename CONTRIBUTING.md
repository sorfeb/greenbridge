# Contributing

Thanks for helping. Bug reports, fixes, and new bridges are all welcome.

## Before you start

- **Bugs:** open an [issue](https://github.com/sorfeb/greenbridge/issues/new/choose) with `gbridge doctor` output.
- **Features:** open an issue first. Anything that changes what a mirror commit carries needs agreement before code, because privacy is the point of `gbridge`.
- **Security issues:** follow [SECURITY.md](SECURITY.md), not the issue tracker.

## Set up

You need Go 1.27 and `git`. Every other dev tool (golangci-lint, Task, gotestsum, govulncheck) is pinned in `tools.mod` and downloads on first use.

```sh
git clone https://github.com/sorfeb/greenbridge
cd greenbridge
go tool -modfile tools.mod task check   # every gate CI runs
```

| Task | Does |
|---|---|
| `task check` | Format check, lint, `go.mod` tidiness, race tests, govulncheck |
| `task fix` | Applies lint and format auto-fixes |
| `task art` | Regenerates the README art and social preview from `internal/brand` |
| `task --list` | Everything else |

Prefix each with `go tool -modfile tools.mod`. Write the flag with a space: PowerShell splits `-modfile=tools.mod` at the dot.

## Make a change

1. Branch from `main` as `<type>/<scope>-<description>`, for example `fix/sync-empty-repo`.
2. Write the test first. Output users see (banner, graph, art) is pinned by golden files and generated assets; regenerate them, never hand-edit.
3. Run `task check` until it passes. A failing gate gets fixed in the code, not by loosening `.golangci.yml`. A `//nolint` names the linter and says why.
4. Commit with [Conventional Commits](https://www.conventionalcommits.org): `feat(sync): skip repos without commits`. Release notes are grouped from these prefixes.
5. Open a pull request. The template asks for a summary, evidence (terminal output for CLI changes), and risk.

Pull requests merge with a merge commit or a rebase, never a squash, so each commit keeps its message in history.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
