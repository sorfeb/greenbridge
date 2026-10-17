// PostToolUse hook (Write|Edit): format .go files with golangci-lint fmt.
// Exit 2 on failure (syntax error) so Claude sees stderr. Skips during merge/rebase.
import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { dirname, isAbsolute, join, resolve } from 'node:path'

function inProgress(file) {
  try {
    const gitDir = execFileSync('git', ['rev-parse', '--absolute-git-dir'], {
      cwd: dirname(file),
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      windowsHide: true,
    }).trim()
    return [
      'MERGE_HEAD',
      'CHERRY_PICK_HEAD',
      'REVERT_HEAD',
      'rebase-merge',
      'rebase-apply',
    ].some((n) => existsSync(join(gitDir, n)))
  } catch {
    return false // not a repo: format anyway
  }
}

let raw = ''
process.stdin.on('data', (c) => (raw += c))
process.stdin.on('end', () => {
  let file = ''
  try {
    file = JSON.parse(raw).tool_input?.file_path || ''
  } catch {
    process.exit(0)
  }
  if (!file.endsWith('.go')) process.exit(0)
  if (!isAbsolute(file)) {
    file = resolve(process.env.CLAUDE_PROJECT_DIR || process.cwd(), file)
  }
  if (!existsSync(file)) process.exit(0)
  if (inProgress(file)) process.exit(0)

  // Same formatter as `task fmt-check` (gofumpt + gci via golangci-lint), so
  // agent edits never fail the gate on layout. Runs from the repo root where
  // tools.mod and .golangci.yml live.
  let root = ''
  try {
    root = execFileSync('git', ['rev-parse', '--show-toplevel'], {
      cwd: dirname(file),
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      windowsHide: true,
    }).trim()
  } catch {
    process.exit(0)
  }
  // golangci-lint fmt skips files it cannot parse without failing, so parse
  // first with gofmt and surface syntax errors to Claude.
  const parse = spawnSync('gofmt', ['-e', '-l', file], { encoding: 'utf8', windowsHide: true })
  if (!parse.error && parse.status !== 0) {
    process.stderr.write(parse.stderr || `gofmt could not parse ${file}\n`)
    process.exit(2)
  }
  const r = spawnSync('go', ['tool', '-modfile=tools.mod', 'golangci-lint', 'fmt', file], {
    cwd: root,
    encoding: 'utf8',
    windowsHide: true,
  })
  if (r.error) process.exit(0) // go missing: fail open
  if (r.status !== 0) {
    process.stderr.write(r.stderr || r.stdout || `golangci-lint fmt failed on ${file}\n`)
    process.exit(2)
  }
  process.exit(0)
})
