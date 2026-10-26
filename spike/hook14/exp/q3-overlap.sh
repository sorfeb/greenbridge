#!/bin/sh
# Q3 follow-up: two pushes while one sync runs. A plain lock skips the second
# sync, so commits made after the first scan wait for the next push.
GBRUN=q3-overlap . "$(dirname "$0")/env.sh"
export GBSPIKE_SYNC_SECS=4 GBSPIKE_MODE=detach
install_config_hook
mkrepo r
commit r && git -C "$RUN/r" push -q origin HEAD
commit r && git -C "$RUN/r" push -q origin HEAD
sleep 5
events | grep -oE '"ev":"[a-z-]+"|"t":"[^"]+"' | paste - - | sort -t'"' -k8
