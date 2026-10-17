// SessionStart + PreToolUse hook: keep sessions honest about how stale the
// shared root is relative to origin/main. Ported from tailorcv.
//
// The root is a shared checkout that agents read facts from and branch
// worktrees off. Two things go wrong silently when it is behind:
//
//   1. Branching from stale main: `git worktree add -b <branch>` forks from
//      whatever the root is parked on. (Claude Code's worktree tool is covered
//      by worktree-create.mjs; this catches the hand-typed command.)
//   2. Asserting stale facts: "that file does not exist" from a tree several
//      merges old.
//
// Always FETCHES. At SessionStart, and before a `git worktree add`, also
// FAST-FORWARDS the root, only when provably safe:
//   - root is on main with no local commits, and
//   - no dirty tracked file is in the incoming change set.
// Every other case falls through to an advisory. Never forces.
//
// Set GREENBRIDGE_NO_AUTO_PULL=1 to keep it fetch-only.
//
// Fails OPEN everywhere: no network, no git, detached HEAD, missing remote.
// The settings.json `if` filter is a spawn-cost optimization only; the hook
// re-checks the command itself.

import { execFileSync } from "node:child_process";

const GIT_TIMEOUT_MS = 10_000;

/** Run git, returning trimmed stdout, or null on any failure (fail open). */
function git(args, cwd) {
  try {
    return execFileSync("git", args, {
      cwd,
      encoding: "utf8",
      timeout: GIT_TIMEOUT_MS,
      stdio: ["ignore", "pipe", "ignore"],
      windowsHide: true,
    }).trim();
  } catch {
    return null;
  }
}

const lines = (out) => (out ?? "").split(/\r?\n/).filter(Boolean);

/** The MAIN worktree (listed first), even when run from a linked worktree. */
function mainWorktree(startCwd) {
  const first = lines(git(["worktree", "list", "--porcelain"], startCwd)).find(
    (l) => l.startsWith("worktree "),
  );
  return first ? first.slice("worktree ".length).trim() : null;
}

/** Commits origin/main has that the root does not. Null when undeterminable. */
function commitsBehind(root) {
  if (git(["rev-parse", "--verify", "--quiet", "origin/main"], root) === null) {
    return null;
  }
  const counts = git(
    ["rev-list", "--left-right", "--count", "origin/main...HEAD"],
    root,
  );
  if (!counts) return null;
  const [behind, ahead] = counts.split(/\s+/).map(Number);
  if (!Number.isFinite(behind) || !Number.isFinite(ahead)) return null;
  return { behind, ahead };
}

/**
 * Fast-forward the root to origin/main when every safety condition holds.
 * Returns { status: "pulled" }, { status: "blocked", clash }, or null when
 * the attempt does not apply. Every git failure returns null.
 */
function tryFastForward(root, state) {
  if (process.env.GREENBRIDGE_NO_AUTO_PULL) return null;
  if (state.ahead !== 0) return null;
  if (git(["symbolic-ref", "--short", "-q", "HEAD"], root) !== "main") return null;

  const incoming = lines(git(["diff", "--name-only", "HEAD..origin/main"], root));
  if (incoming.length === 0) return null;

  // A fast-forward only fails on a dirty file the merge also writes.
  const dirty = new Set(lines(git(["diff", "--name-only", "HEAD"], root)));
  const clash = incoming.filter((f) => dirty.has(f));
  if (clash.length > 0) return { status: "blocked", clash };

  if (git(["merge", "--ff-only", "--quiet", "origin/main"], root) === null) {
    return null;
  }
  return { status: "pulled" };
}

function emit(obj) {
  process.stdout.write(JSON.stringify(obj));
}

let raw = "";
process.stdin.on("data", (c) => (raw += c));
process.stdin.on("end", () => {
  let payload;
  try {
    payload = JSON.parse(raw);
  } catch {
    return;
  }

  const event = payload.hook_event_name ?? "";
  const cwd = payload.cwd || process.cwd();

  // Scope check FIRST, before any git subprocess or network fetch.
  if (
    event === "PreToolUse" &&
    !/git\s+worktree\s+add/.test(String(payload.tool_input?.command ?? ""))
  ) {
    return;
  }

  const root = mainWorktree(cwd);
  if (!root) return;

  git(["fetch", "origin", "main", "--quiet"], root);

  const state = commitsBehind(root);
  if (!state || state.behind === 0) return;

  const shortRoot = git(["rev-parse", "--short", "HEAD"], root) ?? "HEAD";
  const shortMain =
    git(["rev-parse", "--short", "origin/main"], root) ?? "origin/main";
  const plural = state.behind === 1 ? "commit" : "commits";
  const drift = `The shared root (${root}) is ${state.behind} ${plural} behind origin/main (${shortRoot}..${shortMain}).`;

  if (event === "SessionStart") {
    const ff = tryFastForward(root, state);

    if (ff?.status === "pulled") {
      return emit({
        systemMessage: `Fast-forwarded the shared root ${state.behind} ${plural}`,
        hookSpecificOutput: {
          hookEventName: "SessionStart",
          additionalContext: `Updated the shared root (${root}) to origin/main: ${state.behind} ${plural} fast-forwarded (${shortRoot}..${shortMain}). The root had no local commits and no dirty tracked file was in the incoming change set, so the merge could not overwrite anything.`,
        },
      });
    }

    const blockedNote =
      ff?.status === "blocked"
        ? [
            "",
            `An automatic fast-forward was skipped: ${ff.clash.join(", ")} ${ff.clash.length === 1 ? "is" : "are"} modified in the root and also changed upstream, so the merge would have overwritten uncommitted work. Deal with that file, then pull.`,
          ]
        : [];

    return emit({
      hookSpecificOutput: {
        hookEventName: "SessionStart",
        additionalContext: [
          drift,
          "",
          "Before acting on this, pick the branch that matches the work:",
          "- Read-only (triage, audits, answering questions about repo state):",
          "  read origin/main directly. Do NOT pull. Pulling mutates a working tree other",
          "  agents may be editing.",
          "- Committable work: pull the root to main FIRST, then `git worktree add`, so the",
          "  branch forks from current main instead of this stale HEAD.",
          "",
          "A fetch at session start does not stay true. Merges land while a session",
          "runs, so re-fetch before asserting any current fact about the repo.",
          ...blockedNote,
        ].join("\n"),
      },
    });
  }

  if (event === "PreToolUse") {
    const ff = tryFastForward(root, state);
    if (ff?.status === "pulled") {
      return emit({
        systemMessage: `Fast-forwarded the shared root ${state.behind} ${plural} (${shortRoot}..${shortMain}) before the worktree add.`,
        hookSpecificOutput: {
          hookEventName: "PreToolUse",
          additionalContext: `Auto-fast-forwarded the shared root to origin/main (${shortRoot}..${shortMain}) before branching: it had no local commits and no dirty tracked file clashed with the incoming change set.`,
        },
      });
    }

    const why =
      ff?.status === "blocked"
        ? `An automatic fast-forward was refused: ${ff.clash.join(", ")} ${ff.clash.length === 1 ? "is" : "are"} dirty in the root AND changed upstream, so pulling would overwrite uncommitted work.`
        : `An automatic fast-forward does not apply (the root has local commits, is parked off main, or the merge was refused), so someone is likely working in it.`;
    return emit({
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        permissionDecision: "ask",
        permissionDecisionReason: `${drift} ${why} Branching now forks from the stale HEAD unless the command passes an explicit origin/main start-point. Resolve the root's state first, or approve only if the start-point is explicit.`,
      },
    });
  }
});
