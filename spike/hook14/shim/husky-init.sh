# Experiment only: husky v9 sources ~/.config/husky/init.sh before running a
# hook, but only when the repo has a script for that hook. Tees stdin to
# gbridge, then points the shell's stdin back at the copy for the repo hook.
if [ "$n" = pre-push ]; then
	_gb_t=$(mktemp "${TMPDIR:-/tmp}/gbridge-pre-push.XXXXXX") && {
		cat >"$_gb_t"
		${GBRIDGE_HOOK:-gbridge hook} "$@" <"$_gb_t" >/dev/null 2>&1 || :
		exec 0<"$_gb_t"
		rm -f "$_gb_t"
	}
fi
