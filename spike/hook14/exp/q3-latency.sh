#!/bin/sh
# Q3: push latency per hook mode on a big push, and what each mode leaves
# behind when the push fails after the hook ran.
GBRUN=q3 . "$(dirname "$0")/env.sh"
N=${N:-100000}
git --version
install_config_hook

# Big history via fast-import: N commits, one file, a minute apart.
git init -q "$RUN/big"
awk -v n="$N" 'BEGIN{for(i=1;i<=n;i++){t=1600000000+i*60
	printf "commit refs/heads/main\nmark :%d\nauthor spike <spike@example.invalid> %d +0000\ncommitter spike <spike@example.invalid> %d +0000\ndata 2\nc\n",i,t,t
	if(i>1)printf "from :%d\n",i-1
	printf "M 644 inline f.txt\ndata %d\n%d\n\n",length(i)+1,i}}' |
	git -C "$RUN/big" fast-import --quiet
cd "$RUN/big" && git reset -q --hard main || exit 1
echo "history: $(git rev-list --count main) commits"

ms() { echo $((($(date +%s%N) - $1) / 1000000)); }
push_fresh() { # push_fresh MODE: push all N commits to an empty bare remote
	rm -rf "$RUN/big.git" && git init -q --bare "$RUN/big.git"
	git remote remove origin 2>/dev/null; git remote add origin "$RUN/big.git"
	t=$(date +%s%N); GBSPIKE_MODE=$1 git push -q origin main; e=$?
	echo "mode=$1 push exit=$e wall=$(ms "$t")ms"
}
git config hook.gbridge.enabled false; push_fresh off; git config --unset hook.gbridge.enabled
for m in none detach queue inherit; do push_fresh $m; done

echo "--- incremental push of 1 commit"
for m in detach queue; do
	commit big; t=$(date +%s%N); GBSPIKE_MODE=$m git push -q origin main; echo "mode=$m wall=$(ms "$t")ms"
done

echo "--- what the background sync pays instead: source.Scan's git log over all refs"
t=$(date +%s%N); git log --branches --remotes --tags --no-color --format=%H%x1f%ae%x1f%aI | wc -l; echo "scan=$(ms "$t")ms"

echo "--- remote rejects the push after capture ran"
printf '#!/bin/sh\necho "remote: rejected by policy" >&2\nexit 1\n' >"$RUN/big.git/hooks/pre-receive"
chmod +x "$RUN/big.git/hooks/pre-receive"
commit big; commit big
: >"$RUN/gbspike.queue"
GBSPIKE_MODE=queue git push -q origin main; echo "push exit=$?"
echo "queue now holds $(wc -l <"$RUN/gbspike.queue") entries for commits the remote never accepted"
sleep 4
echo "--- hook events (hook_ms, queued, spawn_err)"
events | sed -E 's/"stdin":\[[^]]*\],?//; s/"args":\[[^]]*\],?//'
