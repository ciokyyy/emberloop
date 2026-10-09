#!/usr/bin/env bash
# Control-channel helper for the ticket 07 live probe
# (scripts/input-queue-probe.sh): one round trip to a wrapped agent's unix
# socket. Matches emberloop/inputqueue.go: JSON {"op":...} in, JSON
# {ok,error,pending} out; socket name sanitized like lockName (one dash
# per rejected character), with the FNV-1a hash fallback past 100 bytes.
#
# Usage: probe-ctrl.sh <agent-id> <pending|steer|queue> [text]
# Prints `OK pending=N` or `FAIL - <reason>`. Never runs in the automated
# check.
set -uo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: probe-ctrl.sh <agent-id> <pending|steer|queue> [text]" >&2
  exit 2
fi
command -v python3 >/dev/null || { echo "FAIL - python3 needed for the control-channel helper"; exit 1; }

AGENT_ID="$1" OP="$2" TEXT="${3:-}" python3 - <<'PY'
import json, os, socket
agent = os.environ["AGENT_ID"]
op = os.environ["OP"]
text = os.environ["TEXT"]
name = "".join(c if (c.isascii() and (c.isalnum() or c in "-_")) else "-"
               for c in agent) or "lock"
raw = agent.encode("utf-8")
path = os.path.join(os.path.expanduser("~"), ".pig", "state",
                    "el", "agents", f"ctrl-{name}.sock")
if len(path) > 100:
    h = 14695981039346656037
    for b in raw:
        h ^= b
        h = (h * 1099511628211) % (1 << 64)
    path = os.path.join(os.path.dirname(path), f"ctrl-{h:016x}.sock")
if op not in ("pending", "steer", "queue"):
    print(f"FAIL - unknown op {op!r}: want pending, steer or queue")
    raise SystemExit(2)
req = {"op": "pending"} if op == "pending" else {
    "op": "enqueue", "kind": op, "text": text, "source": "probe-07"}
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.settimeout(3)
try:
    s.connect(path)
except OSError as e:
    print(f"FAIL - no wrapper socket for {agent}: {e}")
    raise SystemExit(1)
s.sendall((json.dumps(req) + "\n").encode())
res = json.loads(s.recv(4096).decode())
if not res.get("ok"):
    print(f"FAIL - channel refused: {res.get('error')}")
    raise SystemExit(1)
print(f"OK pending={res['pending']}")
PY
