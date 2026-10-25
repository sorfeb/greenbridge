---
name: ci-iterate
description: The loop of waiting for CI, reading the red job's log, fixing the code, pushing, and waiting again, until the pull request is green or a stop rule fires. Use when a PR's CI is red, when waiting on CI after pushing a fix, or when asked to keep fixing until the PR passes. Not for opening a pull request (that is the safe-workflow skill), not for deploys, and it never merges; the merge belongs to the user.
---

# CI iterate

One round: wait, read the red job's log, fix, push. Repeat until green or a stop rule fires. Opening the PR and the handover stay in the `safe-workflow` skill; this one expands its step 7. Its "Honestly green" rule applies here: fix the code, not the gate. A gate changes only after the user agrees, in its own commit, named in the PR body.

## Wait

```bash
gh pr checks <number> --watch --interval 20
```

- Exit 0: green. Done; hand over per `safe-workflow` step 8.
- Exit 1: a job failed. Go to "Read the log".
- Exit 8: still pending when the watch ended. Run it again; this is not an iteration.

`No checks reported` right after a push means the run has not registered yet. Wait a minute and retry before concluding anything.

PR CI tests the **merge commit** with `main`, not the branch alone. A branch on a stale base can go green there and still fail the push run after merge. When `main` has moved, rebase onto `origin/main` and let CI run again before handing over.

## Read the log

```bash
gh run list --branch <branch> --limit 5
gh run view <run id> --log-failed
```

Match the run to the head commit (`headSha` in `gh run view <id> --json headSha`), not to the branch name. Read from the bottom: the PR's own error is usually near the end, the first lines are runner setup.

Reproduce locally with the same command the job runs (`.github/workflows/`, usually `go tool -modfile tools.mod task check`) before pushing a fix. A fix pushed without a local reproduction is a guess that costs a CI round.

## Fix and push

- Work in that PR's worktree. One iteration closes one cause of red; do not stack unrelated fixes in the same commit.
- Imperative commit subject, push, back to "Wait".

## Stop and report

Stop when any of these fires, then report to the user:

1. Five iterations on the same PR without green.
2. The same failure returns after the iteration that targeted it: the fix missed the cause, and repeating only burns runner time.
3. The code cannot satisfy the gate: name the job, why it cannot pass, and the gate change you propose.
4. Red from a cause outside the PR that the job's own error names (runner outage, registry timeout, a provider down): leave it red and report.

The report names the PR link, the red job per iteration, the fix commits, and the remaining blocker.

Merging is not part of this loop.
