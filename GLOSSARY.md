# greenbridge

gbridge turns commits in local clones GitHub can't see into empty, backdated commits in a GitHub repo, so the contribution graph shows real work. One term per concept: use these words in code, docs, output, and issues.

## Source side (never leaves the machine)

**Source repo**:
A local git clone you author commits in, listed in config or found in a scan folder.
_Avoid_: work repo, clone, source clone

**Scan folder**:
A folder searched on every plan and sync for source repos below it.
_Avoid_: scan dir, root, scan (alone)

**Activity**:
One commit in a source repo authored under one of your configured emails, on a branch, remote-tracking branch, or tag.
_Avoid_: commit (when meaning the read result), contribution

**Secret**:
The 32 random bytes that key every mirror ID, shown to you once as a recovery phrase and shared by all your machines.
_Avoid_: key, salt, token

## Mirror IDs and entries

**Mirror ID**:
The opaque 12-character ID of one activity: by default a keyed hash of its author email and author time, so it survives rebase, amend, and cherry-pick.
_Avoid_: hash, sha, commit ID

**Entry**:
A mirror ID with the time written for it under the time policy: exact for a private mirror repo, date only for a public one.
_Avoid_: record, row, item

**Desired**:
The set of entries the mirror repo should hold, built from every activity in every source repo, one per mirror ID.
_Avoid_: wanted, expected, source set

**Ledger**:
The set of mirror IDs already in the mirror repo, read back from mirror commit subjects; the only state gbridge keeps.
_Avoid_: state, history, already synced

**Orphan**:
A mirror ID in the ledger that desired on this machine lacks: another machine's work, or work no longer configured. Sync never removes it.
_Avoid_: stale ID, extra

**Plan**:
Desired compared with the ledger: the entries the mirror repo is missing, shown before anything is written.
_Avoid_: diff, preview (as a noun)

## Mirror side (what reaches GitHub)

**Mirror repo**:
Your GitHub repo that receives mirror commits, `greenbridge-activity` by default.
_Avoid_: activity repo, target repo, mirror (alone)

**Mirror clone**:
The local bare clone of the mirror repo that gbridge writes to and pushes from.
_Avoid_: local mirror, mirror (alone)

**Mirror settings**:
The part of config that names the mirror repo, its clone path, branch, and visibility.
_Avoid_: mirror config

**Mirror commit**:
One empty commit in the mirror repo carrying exactly one entry: its mirror ID in the subject and its time as the author date.
_Avoid_: fake commit, activity commit

**Mirror author**:
The identity written on every mirror commit, normally your GitHub noreply address, so GitHub links the commit to your account.
_Avoid_: author (alone), committer

**Contribution**:
A square on your GitHub contribution graph. GitHub alone decides whether a mirror commit becomes one.
_Avoid_: activity, square (in code)

## Automation

**Push hook**:
The pre-push hook that starts a background sync on every git push from this machine: in global git config on Git 2.54 and later, else a script in each source repo's `.git/hooks`. It never fails or slows the push, and changes no tracked file.
_Avoid_: git hook (alone), trigger, shim

**Background sync**:
A sync started by the push hook or `gbridge sync --detach`, detached from the terminal, that writes its output and one result line to `sync.log` in the greenbridge home. One runs at a time.
_Avoid_: detached sync, async sync, daemon

## Diagnosis

**Doctor check**:
One independent precondition for contributions to appear, reported by `gbridge doctor` as ok, warn, unverified, failed, or skipped.
_Avoid_: test, probe, validation

**Online check**:
A doctor check that waits on the network beyond GitHub's API, run only by `gbridge doctor --online`. Plain `gbridge doctor` reports it skipped and stays fast.
_Avoid_: network check, slow check

**Tracked remote**:
`origin`, or a remote a local branch of a source repo tracks. Its new commits reach sync only through `git fetch`. A remote no branch tracks is left out.
_Avoid_: upstream, stale remote
