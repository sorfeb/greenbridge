# greenbridge

Mirror the timing of commits GitHub cannot see (employer GitLab, Bitbucket, Gitea, on-prem servers) onto your GitHub contribution graph. `gb` reads commit metadata from local clones and writes one empty, backdated commit per source commit to a mirror repo you own.

## What leaves your machine

Per source commit: a timestamp (date only for public mirrors) and an opaque 12-character ID, `HMAC(secret, email NUL author_unix)`. Code, diffs, file names, commit messages, repo and branch names, commit hashes, and your work email stay local.

## Install

```sh
go install github.com/sorfeb/greenbridge/cmd/gb@latest
```

Requires `git`, and the [GitHub CLI](https://cli.github.com) logged in (`gh auth login`).

## Use

```sh
gb init --email you@work.com --scan ~/work   # creates a private greenbridge-activity repo
gb plan                                      # preview, writes nothing
gb sync                                      # mirror and push; rerun any time
gb doctor                                    # why squares are missing
```

- `gb sync` is idempotent: it reads already-synced IDs back from the mirror's log. There is no state file, and several machines can sync into one mirror.
- Past work is recovered on the first sync, as far back as your clones go. `--since YYYY-MM-DD` limits it.
- Rebased, cherry-picked, and amended commits keep their author date, so they are not counted twice.
- Repos with a github.com remote are skipped, since GitHub already counts them.
- Private mirror: turn on **Include private contributions on my profile** at <https://github.com/settings/profile>.
- On a second machine, run `gb secret` on the first and `gb init --secret <phrase>` on the second. A different secret produces different IDs and double counts.

`GB_HOME` overrides the config directory.

## Develop

```sh
gofmt -l . && go vet ./... && go test ./...
```

Planning lives on Linear, project greenbridge.
