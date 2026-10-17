---
name: pr-body
description: Use when writing or rewriting a PR body in this repo (`gh pr create`, `gh pr edit --body`). Mandatory - a hook blocks both commands until this skill has loaded. Covers the three sections, which visual fits which change, and how to state risk. Evidence for a CLI is terminal output.
---

# PR body

Write for a reviewer who approves without reading the diff. Show, then tell: each section leads with one plain sentence, then the smallest visual that makes the point. Keep prose brief and skip preambles.

Three sections, in this order, with these headings:

```markdown
## Summary

## Evidence

## Risk and rollback
```

Name the Linear issue on the first line of the Summary (`Fixes SOR-123`, or `Refs SOR-123` for one part of it).

## Summary

One sentence on what changed and why, then a visual. Pick by change type:

| Change                                   | Visual                                                |
| ---------------------------------------- | ----------------------------------------------------- |
| New or changed command, flag, or output  | Before and after of the terminal output               |
| Logic or algorithm                       | Pseudocode, as a `diff` when the shape already exists |
| Runtime control flow                     | Call tree, as a `diff` when calls move                |
| Package structure                        | Component or package tree with boundaries that matter |
| Refactor or file move                    | Shallow file tree `diff`                              |
| Interaction between CLI, mirror, network | Mermaid `sequenceDiagram`                             |
| State or lifecycle rules                 | Mermaid `stateDiagram-v2`                             |

```diff
 runPlan
   scan
+    hmacIdentity
   mirrorSync
```

Place each visual next to the sentence it supports. Keep only the calls, files, and states the reviewer needs. One visual is normal, three is a lot. Most small PRs need none: a single-file fix, a copy edit, or a dependency bump gets a sentence.

A Mermaid diagram derives every node from the diff, never from memory: a confidently wrong diagram is worse than none. Cap it at about 12 nodes. When it styles nodes, pin both `fill` and `color`, or dark mode makes them unreadable.

## Evidence

Show a before and an after, labelled `**Before:**` and `**After:**`.

- CLI change: terminal output in a code block. Paste the `gb plan` or `gb help` output, trimmed to the lines that prove the point. No screenshots.
- Everything else: the exact command and its output. For a fix, the test that failed and now passes.

State the gate that ran (`go tool -modfile tools.mod task check`) and its result. State what was not run and why.

## Risk and rollback

- `**Door:**` two-way when a revert restores the old state with nothing lost; one-way when it changes an on-disk format, an identity scheme (HMAC inputs), or a published interface other tools depend on.
- `**Blast radius:**` one word (isolated, feature, app-wide), then what could break and for whom.

Say when an approved gate changed (`safe-workflow`, "Honestly green").

## Public surface

The PR is on GitHub. Nothing in it names the owner, hints at team size, or carries a secret or a real user's data.

## Credits

Adapted from the `pr` skill in [mattpocock/skills](https://github.com/mattpocock/skills) (MIT), whose menu of visuals comes from Dex Horthy's `show-me` skill ([humanlayer/skills](https://github.com/humanlayer/skills)).
