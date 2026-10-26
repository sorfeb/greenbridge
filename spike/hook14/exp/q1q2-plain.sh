#!/bin/sh
# Q1/Q2 baseline: global config hook plus a classic .git/hooks/pre-push that
# also reads stdin. Who runs, in what order, who sees stdin, whose exit wins.
GBRUN=q1q2-plain . "$(dirname "$0")/env.sh"
export GBSPIKE_MODE=none
git --version
install_config_hook
mkrepo r
cat >"$RUN/r/.git/hooks/pre-push" <<'EOF'
#!/bin/sh
echo "repo-hook: args=$* stdin=[$(cat)] exit=${REPO_EXIT:-0}" >&2
exit ${REPO_EXIT:-0}
EOF
chmod +x "$RUN/r/.git/hooks/pre-push"
cd "$RUN/r" || exit 1

echo "--- git hook list --show-scope pre-push"
git hook list --show-scope pre-push

echo "--- push, repo hook exits 0"
git push -q origin main; echo "push exit=$?"

echo "--- push, repo hook exits 1"
commit r
REPO_EXIT=1 git push -q origin main; echo "push exit=$?"

echo "--- push, our hook exits 1 (why the hard rule exists)"
GBSPIKE_EXIT=1 git push -q origin main; echo "push exit=$?"

echo "--- push --no-verify"
git push -q --no-verify origin main; echo "push exit=$?"

echo "--- per-repo opt-out: hook.gbridge.enabled=false"
commit r
git config hook.gbridge.enabled false
git hook list --show-scope pre-push
git push -q origin main; echo "push exit=$?"

echo "--- gbspike events"
events
