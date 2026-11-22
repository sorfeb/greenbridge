# greenbridge

Rules only. Guardrails live in hooks (`.claude/hooks/`), workflows in skills (`.claude/skills/`), lint policy in `.golangci.yml`. To change enforced behavior, change those, not this prose.

## Commands

All dev tools are pinned in `tools.mod`; nothing to install but Go 1.27.

```sh
go tool -modfile tools.mod task check   # every CI gate: fmt-check, lint, tidy, race tests, govulncheck
go tool -modfile tools.mod task fix     # lint and format auto-fixes
go tool -modfile tools.mod task --list  # everything else
```

## Conventions

- Words for domain concepts come from `GLOSSARY.md`, in code, output, docs, and issues. A new concept gets its entry in the same PR.
- `go.mod` lists only what gbridge ships with. Dev tools go in `tools.mod` via `go get -tool -modfile tools.mod`.
- **Honestly green**: a red gate is fixed in the code, never by weakening the gate. A `//nolint` names the linter and says why.
- Tests first. No tautological tests. Output that users see (banner, graph) is pinned by goldens.
- `internal/brand/testdata/*.golden` are generated from `prototype/brand/bridges.prototype.html` on branch `prototype/brand-ascii-logo`. Change the prototype, regenerate, never hand-edit (a hook blocks it).
- `.github/assets/` (README art, social preview) is generated from `internal/brand/art.go` by `task art`; a test fails when it is stale.
- Nothing about a user's work leaves the machine except timestamps and opaque IDs. Any change to what a mirror commit carries needs an issue first.
- Terminal output degrades to plain text when the stream is not a terminal. Never print color or banners into a pipe.

## Workflow

- Planning lives in GitHub Issues. The pinned roadmap (issue #8) holds the non-negotiables, settled decisions, and the work lanes, each lane owning a set of files. Labels: `area: *`, `priority: *`, plus `refactor`, `research`, `decision`, `launch`. Milestones are releases.
- Name the kind with every number: "issue #15", "PR #7". Issues and PRs share one number space.
- Committable work happens in a worktree from a fresh `origin/main` (the `safe-workflow` skill).
- Conventional commits, `type(scope): subject`. Branches `<type>/<issue>-<description>`, for example `feat/15-hook-installer`; `<type>/<scope>-<description>` when there's no issue.
- PR bodies come from the `pr-body` skill (a hook enforces it). Evidence for a CLI is terminal output.
- **The user tags releases.** Pushing a `v*` tag publishes binaries through `.goreleaser.yaml`; never push a tag unasked. Pull requests run a snapshot build of the same config.
- **The user merges.** Merge commit or rebase-merge, never squash (a hook blocks it).
- No Claude Artifacts. Pages and prototypes are local `.html` files.
