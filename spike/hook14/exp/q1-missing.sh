#!/bin/sh
# Hard rule checks for a broken install on Git 2.54+:
# - gbridge uninstalled, config hook left behind: a bare command blocks every
#   push on the machine; a guarded one-liner does not.
# - event without command (half-written install or uninstall): Git aborts
#   every hook-running command, not just push.
GBRUN=q1-missing . "$(dirname "$0")/env.sh"
git --version
mkrepo r && cd "$RUN/r" || exit 1
git config --global hook.gbridge.command x
git config --global hook.gbridge.event pre-push
for c in 'C:/nowhere/gbridge.exe hook' 'C:/nowhere/gbridge.exe hook >/dev/null 2>&1 </dev/null || :'; do
	git config --global hook.gbridge.command "$c"
	commit r; git push -q origin HEAD; echo "command=[$c] push exit=$?"
done

echo "--- event set, command missing"
git config --global --unset hook.gbridge.command
for c in "commit -q --allow-empty -m x" "push -q origin HEAD" "fetch -q origin" "status -s"; do
	git $c >/dev/null 2>"$RUN/err"; echo "git $c: exit=$? $(head -1 "$RUN/err")"
done
