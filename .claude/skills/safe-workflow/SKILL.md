---
name: safe-workflow
description: The order of work in greenbridge from a Linear issue to an honestly green pull request - issue, worktree, change, local gates, commit, push, PR, CI, handover, post-merge. Use when starting any task or SOR-* issue that changes tracked files, and when a PR's CI is red. Not for read-only work, deploys, or merging; the merge belongs to the user.
---

# Safe workflow

Goal: a PR that is **honestly green**. Every job passes because the code is right, checked by the same gates as `main`.

Repo rules live in `CLAUDE.md`. This page is only the order, and the steps that go wrong.

## Honestly green

A **gate** is anything that decides pass or fail: `.github/workflows/`, `Taskfile.yml`, golangci-lint config, the hooks in `.claude/hooks/`, an assertion in an existing test (including goldens), and any skip, ignore, or exclude list.

- Red job: read that job's log, then fix the code.
- The code cannot satisfy the gate: stop and tell the user. Name the job, why the code cannot pass, and the gate change you propose. Continue after they agree.
- An approved gate change goes in its own commit and is named in the PR body, so a reviewer sees it without reading the diff.
- Never `--no-verify`, `t.Skip`, `//nolint`, or a hand-edited golden to get past a failure. Goldens under `internal/brand/testdata/` are regenerated, never edited (a hook blocks it).
- A red job caused by something outside the PR (a flaky runner, a provider outage): leave it red and report it.

Done when every job is green, or every red job has been reported to the user with its cause.

## Steps

### 1. Issue

Work traces to a Linear issue in the `s11o` team, `greenbridge` project. Set it `In Progress` before creating the worktree, so two sessions do not pick up the same thing. Check for an open PR or a `*sor-<n>*` branch first; continue only when there is none.

A small fix found mid-task (a typo, a broken test) needs no issue. Ask before creating a new issue.

### 2. Worktree from origin/main

The root checkout is shared with the user's dev environment and other sessions. Leave its HEAD alone.

Enter through Claude Code's worktree tool with the branch name as the worktree name: Linear's generated name when the issue has one, else `<type>/<scope>-<description>`. The `WorktreeCreate` hook (`.claude/hooks/worktree-create.mjs`) fetches `origin/main`, branches from it under exactly that name, copies the `.worktreeinclude` files when that file exists, and fails rather than use a stale base.

By hand, the trailing `origin/main` is not optional:

```bash
git fetch origin main
git worktree add .claude/worktrees/<name> -b <type>/<scope>-<description> origin/main
```

### 3. Do the work

Write the test before the code it covers; a test that restates the implementation proves nothing, and a test never encodes a known defect as expected behaviour.

### 4. Local gates

All of these pass before a commit:

```bash
go tool -modfile tools.mod task check
```

If the command fails, fix the cause and rerun. Go files are formatted on every edit by a PostToolUse hook (`gofmt`), but the gate is the authority.

### 5. Commit and push

Atomic commits, each reviewable and revertable alone: for example code, then tests, then docs when they stand apart. Never one commit for everything.

`<type>(<scope>): <imperative subject>`. Body only for a fact the diff cannot show.

Stage by path, never `git add -A`: other sessions share the tree. New commits, never amend a pushed one. Then `git push -u origin <branch>`.

### 6. Pull request

Load the `pr-body` skill and write the body from it. A hook denies `gh pr create` and body edits until it has loaded.

Name the issue in the body (`Fixes SOR-123`) so Linear links it.

### 7. CI

The `ci-iterate` skill holds the loop: wait, read the red job's log, fix, push, wait again, with its stop rules.

### 8. Handover

Give the user the PR link, each job's result, what was not run, and every gate change. **The user merges.** Merge commit or rebase-merge only, never squash (a hook blocks it).

### 9. After the merge

- `git merge-base --is-ancestor <your last sha> origin/main` confirms the merge captured every pushed commit.
- Set the Linear issue `Done`.
- Remove the worktree. After the merge, never at the end of the task.
