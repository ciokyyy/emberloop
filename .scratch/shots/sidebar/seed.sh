#!/usr/bin/env bash
# Seed a throwaway Daemon with sidebar-demo state on the fake backend, then
# leave it running in the foreground. Usage: seed.sh [port] (default 8930).
#
# Sessions are created through the API (fake turns finish in milliseconds),
# then meta.json statuses/titles/parents are patched while the Daemon is down
# and it is restarted: boot loads meta.json verbatim, so failed/working
# states survive. Titles stay ours via EL_SESSION_TITLES=0.
set -euo pipefail
port="${1:-8930}"
root=/tmp/el-sidebar-shots

rm -rf "$root"
mkdir -p "$root/state" "$root/repo-emberloop" "$root/repo-wick"
git -C "$root/repo-emberloop" init -q -b main
git -C "$root/repo-wick" init -q -b main

export EL_SETTINGS="$root/settings.json"
export EL_SESSION_TITLES=0
export EL_MAX_WORKERS=12
cd /home/cioky/Projects/emberloop/emberloop-web
go build -o "$root/emberloop-web" ./cmd/emberloop-web
 "$root/emberloop-web" -addr "127.0.0.1:$port" -state-dir "$root/state" -restart-check 0 &
 daemon=$!
 for i in $(seq 1 50); do
   curl -sf -m 2 "http://127.0.0.1:$port/api/health" >/dev/null && break
   sleep 0.2
 done
base="http://127.0.0.1:$port"
post() { curl -sf -X POST -H 'Content-Type: application/json' -d "$2" "$base$1"; }
get() { curl -sf "$base$1"; }

pa="$(post /api/projects "{\"path\":\"$root/repo-emberloop\"}")"
pb="$(post /api/projects "{\"path\":\"$root/repo-wick\"}")"
pid_a="$(printf '%s' "$pa" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
pid_b="$(printf '%s' "$pb" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"

 mk() { # mk <projectId-or-empty> -> prints session id
   if [ -n "$1" ]; then
     post /api/sessions "{\"harness\":\"fake\",\"model\":\"fake-model\",\"projectId\":\"$1\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'
   else
     post /api/sessions "{\"harness\":\"fake\",\"model\":\"fake-model\",\"workDir\":\"$root\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])'
   fi
 }

s_failed="$(mk "$pid_a")"
s_working="$(mk "$pid_a")"
s_idle="$(mk "$pid_a")"
s_parent_done="$(mk "$pid_a")"
k1="$(mk "$pid_a")"
k2="$(mk "$pid_a")"
s_parent_run="$(mk "$pid_a")"
k3="$(mk "$pid_a")"
s_wick="$(mk "$pid_b")"
s_orphan="$(mk '')"

# One prompt per root so transcripts have content; fake turns settle at once.
for s in "$s_failed" "$s_working" "$s_idle" "$s_parent_done" "$s_parent_run" "$s_wick" "$s_orphan"; do
  post "/api/sessions/$s/prompt" '{"text":"hello sidebar demo"}' >/dev/null
done
sleep 2
kill "$daemon"
wait "$daemon" 2>/dev/null || true

S_FAILED="$s_failed" S_WORKING="$s_working" S_IDLE="$s_idle" \
  S_PDONE="$s_parent_done" K1="$k1" K2="$k2" \
  S_PRUN="$s_parent_run" K3="$k3" \
  S_WICK="$s_wick" S_ORPHAN="$s_orphan" \
  python3 - "$root/state/enderbloom/sessions" <<'EOF'
import json, os, sys

root = sys.argv[1]
env = os.environ
plan = {
    env['S_FAILED']:  dict(status='failed',  title='Fix sidebar disconnect'),
    env['S_WORKING']: dict(status='working', title='Live audio plan'),
    env['S_IDLE']:    dict(status='idle',    title='Old chat'),
    env['S_PDONE']:   dict(status='idle',    title='Release prep'),
    env['K1']:        dict(status='ended',   name='worker-a', parentId=env['S_PDONE']),
    env['K2']:        dict(status='ended',   name='find-dead-code', parentId=env['S_PDONE'], scout=True),
    env['S_PRUN']:    dict(status='working', title='Big feature'),
    env['K3']:        dict(status='working', name='api-scout', parentId=env['S_PRUN']),
    env['S_WICK']:    dict(status='idle',    title='Rename pass'),
    env['S_ORPHAN']:  dict(status='idle',    title='Scratch note', projectId=''),
}
for sid, patch in plan.items():
    p = os.path.join(root, sid, 'meta.json')
    with open(p) as f:
        meta = json.load(f)
    meta.update(patch)
    with open(p, 'w') as f:
        json.dump(meta, f)
    print(f'{sid[:8]} -> {patch}')
EOF

cat <<EOF

Seeded. Starting the Daemon in the foreground:
  UI:      http://127.0.0.1:$port/
  Tiers:   emberloop project shows Needs you (Fix sidebar disconnect),
           Working (Live audio plan, Big feature), Recent (Old chat, Release prep)
  Nesting: Release prep collapsed by default (both kids ended);
           Big feature expanded (kid still working)
  Drawer:  shrink below 768px, open the top-bar menu; /runs shows the Runs drawer
EOF
exec "$root/emberloop-web" -addr "127.0.0.1:$port" -state-dir "$root/state" -restart-check 0
