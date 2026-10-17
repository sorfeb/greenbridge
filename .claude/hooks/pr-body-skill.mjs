// PreToolUse hook: a PR body is written with the `pr-body` skill loaded.
//
//   - `gh pr create`, or `gh pr edit` that sets a body, with no `pr-body`
//     skill load in this session's transcript -> DENY, naming the skill.
//   - anything else, or any failure to read the payload or transcript
//     -> no opinion (exit silently). A writing guide never blocks other work.
//
// Reads the hook payload JSON on stdin, writes a permissionDecision JSON on
// stdout. Ported from the tailorcv repo.
import { readFileSync } from 'node:fs'

// Either the Skill tool call or a user-typed /pr-body leaves one of these.
const SKILL_LOADED_RE =
  /"skill"\s*:\s*"(?:[\w-]+:)?pr-body"|<command-name>\/?(?:[\w-]+:)?pr-body</

let raw = ''
process.stdin.on('data', (c) => (raw += c))
process.stdin.on('end', () => {
  let payload
  try {
    payload = JSON.parse(raw)
  } catch {
    return
  }

  const cmd = String(payload.tool_input?.command ?? '')
  const creates = /\bgh\s+pr\s+create\b/.test(cmd)
  const editsBody =
    /\bgh\s+pr\s+edit\b/.test(cmd) &&
    /(^|\s)(--body|--body-file|-b|-F)\b/.test(cmd)
  if (!creates && !editsBody) return

  let transcript
  try {
    transcript = readFileSync(payload.transcript_path, 'utf8')
  } catch {
    return
  }
  if (SKILL_LOADED_RE.test(transcript)) return

  process.stdout.write(
    JSON.stringify({
      hookSpecificOutput: {
        hookEventName: 'PreToolUse',
        permissionDecision: 'deny',
        permissionDecisionReason:
          'Load the `pr-body` skill (Skill tool, skill: "pr-body") and write the PR body from it, then re-run this command. See the `safe-workflow` skill, step 6.',
      },
    }),
  )
})
