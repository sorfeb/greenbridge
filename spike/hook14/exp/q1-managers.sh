#!/bin/sh
# Q1: global config hook in repos where husky or lefthook own the hooks.
GBRUN=q1-managers . "$(dirname "$0")/env.sh"
export GBSPIKE_MODE=none
NPM=$GBS/npm/node_modules
LEFTHOOK=$NPM/lefthook-windows-x64/bin/lefthook.exe
git --version
install_config_hook

# The repo's own pre-push body: log stdin, exit $REPO_EXIT.
REPO_BODY='echo "repo-hook($MGR): stdin=[$(cat)] exit=${REPO_EXIT:-0}" >&2; exit ${REPO_EXIT:-0}'

try() { # try LABEL: push twice, repo hook passing then failing
	echo "--- $1: core.hooksPath=$(git config --local core.hooksPath || echo '<unset>')"
	git hook list --show-scope pre-push
	commit "$(basename "$PWD")"
	git push -q origin HEAD; echo "push exit=$? (repo hook 0)"
	commit "$(basename "$PWD")"
	REPO_EXIT=1 git push -q origin HEAD; echo "push exit=$? (repo hook 1)"
}

# husky v9: `husky` sets core.hooksPath=.husky/_ and stubs call .husky/pre-push.
mkrepo husky && cd "$RUN/husky" || exit 1
node "$NPM/husky/bin.js" >/dev/null
printf '%s\n' "MGR=husky; $REPO_BODY" >.husky/pre-push
MGR=husky try husky

# lefthook, default install: writes into the effective hooks dir (.git/hooks).
mkrepo lh && cd "$RUN/lh" || exit 1
printf '%s\n' "MGR=lefthook; $REPO_BODY" >repo-pre-push.sh
printf 'pre-push:\n  jobs:\n    - name: repo\n      run: sh repo-pre-push.sh\n' >lefthook.yml
"$LEFTHOOK" install 2>&1 | tail -3
try lefthook-default

# lefthook in a repo with a local core.hooksPath: refuses unless --force.
mkrepo lh2 && cd "$RUN/lh2" || exit 1
git config core.hooksPath .githooks
cp "$RUN/lh/lefthook.yml" "$RUN/lh/repo-pre-push.sh" .
"$LEFTHOOK" install 2>&1 | grep -m1 "core.hooksPath is set"; "$LEFTHOOK" install --force >/dev/null 2>&1
ls .githooks
try lefthook-hookspath

echo "--- gbspike events (one per push expected: 6)"
events | wc -l
events | sed -E 's/.*"stdin":(\[[^]]*\]).*/\1/'
