#!/bin/sh
# Q4: does a child that keeps our stdout/stderr make the runner wait?
# Runners: Git itself (serial and parallel hooks), pre-commit, prek.
# Also: what console a detached child's own subprocesses get on Windows.
GBRUN=q4 . "$(dirname "$0")/env.sh"
export GBSPIKE_SYNC_SECS=5
PY=$GBS/py/venv/Scripts
git --version; "$PY/pre-commit" --version; "$PY/prek" --version

ms() { echo $((($(date +%s%N) - $1) / 1000000)); }
timed_push() { # timed_push LABEL REPO MODE
	commit "$2"
	t=$(date +%s%N); GBSPIKE_MODE=$3 git -C "$RUN/$2" push -q origin HEAD >/dev/null 2>&1
	echo "$1 mode=$3 push exit=$? wall=$(ms "$t")ms (fake sync runs ${GBSPIKE_SYNC_SECS}s)"
	sleep 6 # let the sync finish so the lock is free
}

echo "--- Git runs the config hook directly"
install_config_hook
mkrepo g
timed_push git-serial g inherit
timed_push git-serial g detach
git -C "$RUN/g" config hook.jobs 2
git -C "$RUN/g" config hook.gbridge.parallel true
timed_push git-parallel g inherit
timed_push git-parallel g detach
git config --global --unset-all hook.gbridge.event # managers only from here

cfg() { # cfg REPO: pre-commit/prek config running gbspike at pre-push
	cat >"$RUN/$1/.pre-commit-config.yaml" <<EOF
default_install_hook_types: [pre-push]
repos:
  - repo: local
    hooks:
      - id: gbspike
        name: gbspike
        entry: $GBSPIKE hook
        language: system
        stages: [pre-push]
        always_run: true
        pass_filenames: false
EOF
	git -C "$RUN/$1" add .pre-commit-config.yaml
}

echo "--- pre-commit"
mkrepo pc && cfg pc && (cd "$RUN/pc" && "$PY/pre-commit" install)
timed_push pre-commit pc inherit
timed_push pre-commit pc detach

echo "--- prek"
mkrepo pk && cfg pk && (cd "$RUN/pk" && "$PY/prek" install)
timed_push prek pk inherit
timed_push prek pk detach

echo "--- pre-commit with a global core.hooksPath (the <2.54 shim)"
git config --global core.hooksPath "$RUN/shim"
mkrepo pc2 && cfg pc2 && (cd "$RUN/pc2" && "$PY/pre-commit" install; echo "exit=$?")
git config --global --unset core.hooksPath

echo "--- console of the sync's subprocesses, by creation flags"
git config --global --add hook.gbridge.event pre-push
GBSPIKE_FLAGS=detached timed_push flags=DETACHED_PROCESS g detach
timed_push flags=CREATE_NO_WINDOW g detach

echo "--- events"
events | grep -E '"ev":"(hook|probe|sync-start)"' | sed -E 's/"args":\[[^]]*\],?//; s/"spawn_err":"",?//'
