// PreToolUse guardrails: deterministic enforcement of rules prose can only
// request. Exit 2 blocks the tool call and feeds stderr back to Claude.
let raw = ''
process.stdin.on('data', (c) => (raw += c))
process.stdin.on('end', () => {
  let input
  try {
    input = JSON.parse(raw)
  } catch {
    process.exit(0)
  }
  const tool = input.tool_name
  const ti = input.tool_input || {}

  // 1. Brand goldens are generated, never hand-edited.
  if (
    (tool === 'Edit' || tool === 'Write' || tool === 'NotebookEdit') &&
    /(^|[\\/])internal[\\/]brand[\\/]testdata[\\/]/.test(ti.file_path || '')
  ) {
    console.error(
      'Blocked: goldens are generated from prototype/brand/bridges.prototype.html on branch prototype/brand-ascii-logo; regenerate, do not hand-edit',
    )
    process.exit(2)
  }

  // 2. Squash merges strand commits. Merge commit or rebase-merge only. If the
  //    user explicitly asked for a squash, they run it themselves.
  const cmd = ti.command || ''
  if (
    (tool === 'Bash' || tool === 'PowerShell') &&
    (/\bmerge\b[^\n]*--squash/.test(cmd) ||
      /\bgh\s+pr\s+merge\b[^\n]*(\s-s\b|--squash)/.test(cmd))
  ) {
    console.error(
      'Blocked: squash merges are banned in this repo (they strand later commits). Use `gh pr merge --merge` or a rebase-merge. If the user explicitly requested a squash, ask them to run it themselves.',
    )
    process.exit(2)
  }

  process.exit(0)
})
