#!/bin/sh
# Q5 alternative: no global core.hooksPath. gbridge writes .git/hooks/pre-push
# into each repo it already mirrors. What happens when a manager installs later?
# Shebang is "#!/usr/bin/env sh": pre-commit on Windows resolves shebangs itself
# and fails the push with "Executable `/bin/sh` not found" for "#!/bin/sh".
GBRUN=q5-perrepo . "$(dirname "$0")/env.sh"
export GBSPIKE_MODE=none
LEFTHOOK=$GBS/npm/node_modules/lefthook-windows-x64/bin/lefthook.exe
git --version
n_events() { [ -f "$GBSPIKE_LOG" ] && grep -c '"ev":"hook"' "$GBSPIKE_LOG" || echo 0; }
ours() { printf '#!/usr/bin/env sh\n%s "$@" >/dev/null 2>&1 || :\n' "$GBSPIKE hook" >"$RUN/$1/.git/hooks/pre-push"; }

mkrepo lh && ours lh && cd "$RUN/lh" || exit 1
printf 'pre-push:\n  jobs:\n    - name: repo\n      run: echo lefthook-ran\n' >lefthook.yml
NO_COLOR=1 "$LEFTHOOK" install >/dev/null 2>&1; ls .git/hooks | grep pre-push
before=$(n_events); commit lh; NO_COLOR=1 git push -q origin HEAD 2>&1 | grep -o lefthook-ran
echo "lefthook installed after us: new events=$(($(n_events) - before))"

mkrepo pc && ours pc && cd "$RUN/pc" || exit 1
printf 'repos:\n  - repo: local\n    hooks:\n      - id: echo\n        name: pc-ran\n        entry: "true"\n        language: system\n        stages: [pre-push]\n        always_run: true\n        pass_filenames: false\n' >.pre-commit-config.yaml
git add .pre-commit-config.yaml
"$GBS/py/venv/Scripts/pre-commit" install -t pre-push; ls .git/hooks | grep pre-push
before=$(n_events); commit pc; git push -q origin HEAD 2>&1 | grep -o 'pc-ran.*Passed'
echo "pre-commit installed after us: new events=$(($(n_events) - before))"
