# Sourced by every experiment. Isolates Git from the user's config.
#   GBGIT=/path/to/git/cmd  portable Git to use (unset: system Git)
#   GBRUN=name              scratch dir name under $GBS/run
GBS=${GBS:-/c/Users/Soros/AppData/Local/Temp/gb-spike14}
RUN=$GBS/run/$GBRUN
rm -rf "$RUN" && mkdir -p "$RUN/xdg"
[ -n "$GBGIT" ] && export PATH="$GBGIT:$PATH"
export GIT_CONFIG_GLOBAL=$RUN/gitconfig GIT_CONFIG_NOSYSTEM=1 XDG_CONFIG_HOME=$RUN/xdg
export GBSPIKE_LOG=$RUN/gbspike.log GBSPIKE_SYNC_SECS=${GBSPIKE_SYNC_SECS:-3}
GBSPIKE=$(cygpath -m "$GBS/bin/gbspike.exe" 2>/dev/null || echo "$GBS/bin/gbspike")
git config --global user.name spike
git config --global user.email spike@example.invalid
git config --global init.defaultBranch main
git config --global core.autocrlf false

# mkrepo NAME: work repo $RUN/NAME with one commit, origin = local bare repo.
mkrepo() {
	git init -q --bare "$RUN/$1.git"
	git init -q "$RUN/$1"
	git -C "$RUN/$1" remote add origin "$RUN/$1.git"
	git -C "$RUN/$1" commit -q --allow-empty -m init
}

# commit REPO: one more empty commit.
commit() { date +%s%N >>"$RUN/$1/f.txt" && git -C "$RUN/$1" add f.txt && git -C "$RUN/$1" commit -q -m "c$(date +%s%N)"; }

# install_config_hook: the Git 2.54+ global config hook under test.
install_config_hook() {
	git config --global hook.gbridge.command "$GBSPIKE hook"
	git config --global hook.gbridge.event pre-push
}

# events: compact view of the spike log.
events() {
	[ -f "$GBSPIKE_LOG" ] || { echo "(no gbspike events)"; return; }
	sed -E 's/"git_env":\[[^]]*\],?//; s/"cwd":"[^"]*",?//' "$GBSPIKE_LOG"
}
