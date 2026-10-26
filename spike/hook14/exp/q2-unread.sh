#!/bin/sh
# Q2 follow-up: a config hook that never reads stdin, on a push whose ref list
# (~300 KB) overflows the pipe buffer. Does Git block or fail on EPIPE?
GBRUN=q2-unread . "$(dirname "$0")/env.sh"
git --version
git config --global hook.gbridge.command true
git config --global hook.gbridge.event pre-push
mkrepo r && cd "$RUN/r" || exit 1
head=$(git rev-parse HEAD)
i=0; while [ $i -lt 3000 ]; do echo "create refs/tags/t$(printf %05d $i) $head"; i=$((i + 1)); done | git update-ref --stdin
printf '#!/bin/sh\nwc -c >&2\n' >.git/hooks/pre-push && chmod +x .git/hooks/pre-push
t=$(date +%s%N); git push -q origin --tags main; echo "push exit=$? wall=$((($(date +%s%N) - t) / 1000000))ms (bytes above: what the repo hook still received)"
