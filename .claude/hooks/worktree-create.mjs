// WorktreeCreate hook: start every agent worktree from a fresh origin/main.
//
//   echo '{"name": "fix/sor-123-login"}' | node .claude/hooks/worktree-create.mjs
//
// Claude Code's own worktree branches from whatever origin/main last held
// locally and names the branch `worktree-<name>`. This fetches first and fails
// rather than fall back to a stale base, and
// the branch takes the name as given, so `fix/sor-123-login` follows the
// repo's branch convention; the directory flattens `/` to `+`.
//
// The hook replaces Claude Code's handling of `.worktreeinclude`, so it copies
// those files itself. Prints the worktree path as the only stdout line, which
// is what Claude Code reads. Ported from the tailorcv repo.
import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'

const REMOTE = 'origin'
const BASE = 'main'

function git(cwd, ...args) {
  try {
    return execFileSync('git', args, {
      cwd,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
      windowsHide: true,
    }).trim()
  } catch (err) {
    throw new Error(
      `git ${args.join(' ')}: ${String(err.stderr || err.message).trim()}`,
    )
  }
}

/** The checkout that owns `.claude/worktrees/`, even when called from a worktree. */
function mainCheckout(cwd) {
  const common = git(
    cwd,
    'rev-parse',
    '--path-format=absolute',
    '--git-common-dir',
  )
  return resolve(dirname(common))
}

function checkName(name) {
  if (
    !name ||
    name.startsWith('/') ||
    name.includes('\\') ||
    name.split('/').includes('..')
  ) {
    throw new Error(
      `worktree name ${JSON.stringify(name)} would leave .claude/worktrees/`,
    )
  }
}

function copyIncluded(root, path) {
  const listing = join(root, '.worktreeinclude')
  if (!existsSync(listing)) return
  for (const raw of readFileSync(listing, 'utf8').split(/\r?\n/)) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) continue
    const source = join(root, line)
    if (!existsSync(source)) continue
    mkdirSync(dirname(join(path, line)), { recursive: true })
    copyFileSync(source, join(path, line))
  }
}

function create(cwd, name) {
  checkName(name)
  const root = mainCheckout(cwd)
  const path = resolve(root, '.claude', 'worktrees', name.replaceAll('/', '+'))
  if (existsSync(path)) return path
  try {
    git(root, 'fetch', '--quiet', REMOTE, BASE)
  } catch (err) {
    throw new Error(
      `cannot fetch ${REMOTE}/${BASE}, refusing a stale base: ${err.message}`,
    )
  }
  if (git(root, 'branch', '--list', name)) {
    git(root, 'worktree', 'add', '--quiet', path, name)
  } else {
    git(
      root,
      'worktree',
      'add',
      '--quiet',
      '--no-track',
      '-b',
      name,
      path,
      `${REMOTE}/${BASE}`,
    )
  }
  copyIncluded(root, path)
  return path
}

let raw = ''
process.stdin.on('data', (c) => (raw += c))
process.stdin.on('end', () => {
  try {
    const name = JSON.parse(raw).name ?? ''
    console.log(create(process.cwd(), name))
  } catch (err) {
    console.error(`worktree-create: ${err.message}`)
    process.exit(1)
  }
})
