#!/bin/sh
# Q5: Git < 2.54. Global core.hooksPath -> chaining shim that tees stdin.
# Run with system Git (2.51), GBGIT unset.
GBRUN=q5 . "$(dirname "$0")/env.sh"
export GBSPIKE_MODE=none
SRC=$(cd "$(dirname "$0")/../shim" && pwd)
NPM=$GBS/npm/node_modules
LEFTHOOK=$NPM/lefthook-windows-x64/bin/lefthook.exe
git --version
mkdir -p "$RUN/shim"
cp "$SRC/pre-push" "$RUN/shim/"
for h in pre-commit commit-msg post-checkout; do cp "$SRC/chain" "$RUN/shim/$h"; done
git config --global core.hooksPath "$RUN/shim"
export GBRIDGE_HOOK="$GBSPIKE hook"

REPO_HOOK='#!/bin/sh
echo "repo-hook: stdin=[$(cat)] exit=${REPO_EXIT:-0}" >&2; exit ${REPO_EXIT:-0}'
n_events() { [ -f "$GBSPIKE_LOG" ] && grep -c '"ev":"hook"' "$GBSPIKE_LOG" || echo 0; }

echo "--- plain repo with .git/hooks/pre-push and pre-commit"
mkrepo r && cd "$RUN/r" || exit 1
printf '%s\n' "$REPO_HOOK" >.git/hooks/pre-push
printf '#!/bin/sh\necho "repo pre-commit ran" >&2\n' >.git/hooks/pre-commit
chmod +x .git/hooks/pre-push .git/hooks/pre-commit
commit r; git push -q origin HEAD; echo "push exit=$? events=$(n_events)"
commit r; REPO_EXIT=1 git push -q origin HEAD; echo "push exit=$? events=$(n_events)"
events | tail -1 | grep -o '"stdin":\[[^]]*\]'

echo "--- same repo, shim without the pre-commit stub"
rm "$RUN/shim/pre-commit"; commit r; echo "(no 'repo pre-commit ran' above means it was hidden)"
cp "$SRC/chain" "$RUN/shim/pre-commit"

echo "--- husky repo (local core.hooksPath=.husky/_)"
mkrepo h && cd "$RUN/h" || exit 1
node "$NPM/husky/bin.js" >/dev/null
echo 'echo "husky pre-commit ran" >&2' >.husky/pre-commit
before=$(n_events); commit h; git push -q origin HEAD; echo "no .husky/pre-push: push exit=$? new events=$(($(n_events) - before))"

echo "--- husky + ~/.config/husky/init.sh tee"
mkdir -p "$XDG_CONFIG_HOME/husky" && cp "$SRC/husky-init.sh" "$XDG_CONFIG_HOME/husky/init.sh"
before=$(n_events); commit h; git push -q origin HEAD; echo "no .husky/pre-push: push exit=$? new events=$(($(n_events) - before))"
printf '%s\n' "$REPO_HOOK" | tail -1 >.husky/pre-push
before=$(n_events); commit h; git push -q origin HEAD; echo "with .husky/pre-push: push exit=$? new events=$(($(n_events) - before))"
before=$(n_events); commit h; REPO_EXIT=1 git push -q origin HEAD; echo "with .husky/pre-push, repo hook 1: push exit=$? new events=$(($(n_events) - before))"
events | tail -1 | grep -o '"stdin":\[[^]]*\]'

echo "--- lefthook install with the global core.hooksPath set"
mkrepo lh && cd "$RUN/lh" || exit 1
printf 'pre-push:\n  jobs:\n    - name: repo\n      run: echo lefthook-pre-push\n' >lefthook.yml
NO_COLOR=1 "$LEFTHOOK" install >"$RUN/lh.out" 2>&1; echo "install exit=$?"; grep -m1 hooksPath "$RUN/lh.out"
ls "$RUN/shim"; head -3 "$RUN/shim/pre-push"

echo "--- pre-commit install with the global core.hooksPath set"
mkrepo pc && cd "$RUN/pc" || exit 1
"$GBS/py/venv/Scripts/pre-commit" install -t pre-push; echo "exit=$?"

echo "--- managers installed before the shim: chained through .git/hooks"
git config --global --unset core.hooksPath
mkrepo lh2 && cd "$RUN/lh2" || exit 1
printf 'pre-push:\n  jobs:\n    - name: repo\n      run: echo lefthook-pre-push-ran\n' >lefthook.yml
NO_COLOR=1 "$LEFTHOOK" install >/dev/null 2>&1
mkrepo pc2 && cd "$RUN/pc2" || exit 1
printf 'repos:\n  - repo: local\n    hooks:\n      - id: echo\n        name: pre-commit-pre-push-ran\n        entry: "true"\n        language: system\n        stages: [pre-push]\n        always_run: true\n        pass_filenames: false\n' >.pre-commit-config.yaml
git add .pre-commit-config.yaml
"$GBS/py/venv/Scripts/pre-commit" install -t pre-push >/dev/null
git config --global core.hooksPath "$RUN/shim"
for r in lh2 pc2; do
	before=$(n_events); commit $r
	NO_COLOR=1 git -C "$RUN/$r" push -q origin HEAD 2>&1 | grep -oE 'lefthook-pre-push-ran|pre-commit-pre-push-ran.*Passed' | head -1
	echo "$r: new events=$(($(n_events) - before))"
done
