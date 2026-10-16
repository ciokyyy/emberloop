#!/usr/bin/env bash
# Live end-to-end probe for Designer Run ticket 15. Uses real model calls.
# Requires go, git, curl, python3, Linux `/proc`, agent-browser, Chromium/Chrome,
# and an authenticated Claude or Pig Backend. Never run by .emberloop/check.
# Usage:
#   EL_LIVE_DESIGNER_RUN=1 EL_LIVE_DESIGNER_HARNESS=claude \
#     EL_LIVE_DESIGNER_CODE_MODEL=haiku EL_LIVE_DESIGNER_MODEL=sonnet \
#     scripts/designer-run-probe.sh
# Pig may use one shared model: EL_LIVE_DESIGNER_HARNESS=pig and set both
# model variables to provider/model. The first failure is reported by step.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

fail() { echo "FAIL - $1" >&2; exit 1; }
[ "${EL_LIVE_DESIGNER_RUN:-}" = 1 ] || { echo "SKIP - set EL_LIVE_DESIGNER_RUN=1 to run the live Designer probe" >&2; exit 2; }
HARNESS="${EL_LIVE_DESIGNER_HARNESS:-claude}"
CODE_MODEL="${EL_LIVE_DESIGNER_CODE_MODEL:-${EL_LIVE_DESIGNER_MODEL:-}}"
DESIGN_MODEL="${EL_LIVE_DESIGNER_MODEL:-}"
[ -n "$CODE_MODEL" ] || fail "set EL_LIVE_DESIGNER_CODE_MODEL (or EL_LIVE_DESIGNER_MODEL)"
[ -n "$DESIGN_MODEL" ] || fail "set EL_LIVE_DESIGNER_MODEL"
case "$HARNESS" in
  claude) command -v node >/dev/null && command -v npx >/dev/null || fail "Claude probe needs node and npx" ;;
  pig) ;;
  *) fail "EL_LIVE_DESIGNER_HARNESS must be claude or pig" ;;
esac
browser_bin="${EL_BIN_DIR:-$HOME/.emberloop/bin}/agent-browser"
[ -x "$browser_bin" ] || fail "install agent-browser with el deps (expected $browser_bin)"
for bin in go git curl python3; do command -v "$bin" >/dev/null || fail "need $bin on PATH"; done
source "$(dirname "${BASH_SOURCE[0]}")/run-upgrade-probe-lib.sh"
TIMEOUT="${EL_LIVE_DESIGNER_TIMEOUT:-1800}"
probe_init designer-run || fail "isolated Daemon setup"
repo="$PROBE_DIR/repo"
probe_git_init "$repo"
probe_add_green_check "$repo"
mkdir -p "$repo/.scratch/designer-probe/issues" "$repo/.emberloop"
cat >"$repo/AGENTS.md" <<'AGENTS'
# Designer probe fixture

Use `.emberloop/preview` to launch the local page. It reads `PORT`, prints its URL, and must be stopped when the Session ends.

## Child DOX Index

No child docs.
AGENTS
cat >"$repo/.emberloop/preview" <<'PREVIEW'
#!/usr/bin/env bash
set -eu
cd "$(dirname "$0")/.."
exec go run ./preview-server.go
PREVIEW
cat >"$repo/preview-server.go" <<'GO'
package main
import("net/http";"os")
func main(){ port:=os.Getenv("PORT"); if port=="" { port="4173" }; http.Handle("/",http.FileServer(http.Dir("."))); _=http.ListenAndServe("127.0.0.1:"+port,nil) }
GO
chmod +x "$repo/.emberloop/preview"
cat >"$repo/.scratch/designer-probe/spec.md" <<'SPEC'
# Designer live probe

A small real-agent run checks code then visual design, using a disposable web page. Complete both tickets and merge them. For design, use the repo's `.emberloop/preview` with the assigned `PORT`; inspect a snapshot first, then capture both 390px and desktop JPEG screenshots, and ensure the page works on phone and desktop. Use the injected Designer skill and submit normally. Reviewers must Pass based on the ticket contract.
SPEC
cat >"$repo/.scratch/designer-probe/issues/01-code.md" <<'TICKET'
# 01: Code page

**Type:** code
**Blocked by:** None

**Status:** open

- [ ] Create `index.html` as a semantic, responsive page with an `h1` containing `Harbor light`, paragraph containing `Beacon 7Q guides ships home.`, and a button labeled `View the harbor`. Keep it plain and functional; do not add visual styling.
TICKET
cat >"$repo/.scratch/designer-probe/issues/02-design.md" <<'TICKET'
# 02: Design page

**Type:** design
**Blocked by:** 01-code

**Status:** open

- [ ] Restyle the existing page as a polished harbor beacon landing page. Preserve the exact heading, paragraph, and button text and behavior. Make it attractive and usable at phone and desktop widths.
TICKET
probe_git_commit "$repo" 'designer probe fixture'
spec="$repo/.scratch/designer-probe/spec.md"
step='run creation and preview types'
echo "starting real $HARNESS Run: code=$CODE_MODEL design=$DESIGN_MODEL timeout=${TIMEOUT}s" >&2
body="$(python3 -c 'import json,sys; print(json.dumps({"spec":sys.argv[1],"repo":sys.argv[2],"harness":sys.argv[3],"model":sys.argv[4]}))' "$spec" "$repo" "$HARNESS" "$CODE_MODEL")"
run="$(probe_api_post /api/runs "$body")" || fail "$step: POST /api/runs"
runid="$(printf '%s' "$run" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"]["id"])')" || fail "$step: missing Run id"
probe_track_run "$runid"
getrun(){ probe_api_get "/api/runs/$runid"; }
printf '%s' "$(getrun)" | python3 -c 'import json,sys; ts={t["id"]:t for t in json.load(sys.stdin)["run"]["tickets"]}; assert ts["01-code"].get("type")=="code",ts["01-code"]; assert ts["02-design"].get("type")=="design",ts["02-design"]; assert "01-code" in ts["02-design"].get("blockers",[]),ts["02-design"]; print("PASS - preview contains code and blocked design Tickets")' || fail "$step"

echo "waiting for Run merge (up to ${TIMEOUT}s); run=$runid" >&2
deadline=$((SECONDS+TIMEOUT))
last_step='Run completion'
while [ "$SECONDS" -lt "$deadline" ]; do
  snapshot="$(getrun 2>/dev/null || true)"
  if [ -n "$snapshot" ]; then
    status="$(printf '%s' "$snapshot" | python3 -c 'import json,sys; print(json.load(sys.stdin)["run"].get("status",""))' 2>/dev/null || true)"
    case "$status" in failed|needs-you|cancelled|abandoned) printf '%s' "$snapshot" >"$PROBE_DIR/failed-run.json"; fail "Run entered $status; artifacts $PROBE_DIR";; esac
    if [ "$status" = done ]; then break; fi
    # Keep any failure label from the active state for useful first-step reporting.
    last_step="$(printf '%s' "$snapshot" | python3 -c 'import json,sys; r=json.load(sys.stdin)["run"]; ts={t["id"]:t for t in r.get("tickets",[])}; c=ts.get("01-code",{}); d=ts.get("02-design",{}); print("code Worker routing" if not c.get("workerSession") else "code Reviewer starts" if len(c.get("reviewerSessions",{})) < 2 else "blocked design scheduling" if not d.get("workerSession") else "design Reviewer starts" if len(d.get("reviewerSessions",{})) < 3 else "design browser checks / Passes")' 2>/dev/null || echo 'Run state could not be decoded')"
  fi
  sleep 3
done
[ "${status:-}" = done ] || fail "$last_step: Run did not merge within ${TIMEOUT}s (run $runid; artifacts $PROBE_DIR)"
snapshot="$(getrun)" || fail "$step: cannot read Run"
printf '%s' "$snapshot" | python3 -c '
import json,sys
r=json.load(sys.stdin)["run"]; ts={t["id"]:t for t in r["tickets"]}; code=ts["01-code"]; design=ts["02-design"]
assert code.get("type")=="code" and design.get("type")=="design",ts
assert "worker" in code.get("chain",{}) and "worker-design" in design.get("chain",{}),(code.get("chain"),design.get("chain"))
assert set(code.get("gate",{}).get("required",[]))=={"reviewer-spec","reviewer-standards"},code
assert set(design.get("gate",{}).get("required",[]))=={"reviewer-spec","reviewer-standards","reviewer-design"},design
assert {a.get("role") for a in code.get("agents",[])} >= {"worker","reviewer-spec","reviewer-standards"},code.get("agents")
assert {a.get("role") for a in design.get("agents",[])} >= {"worker","reviewer-spec","reviewer-standards","reviewer-design"},design.get("agents")
assert set(design.get("gate",{}).get("verdicts",{}))==set(design.get("gate",{}).get("required",[])),design.get("gate")
assert all(v.get("verdict")=="pass" and v.get("fresh") for v in design.get("gate",{}).get("verdicts",{}).values()),design.get("gate")
assert design.get("state")=="merged",design
print("PASS - code/design routing, reviewer sets and fresh three-Pass design gate")' || fail "$step"

step='worker roles and injected skill / browser evidence'
python3 - "$PROBE_BASE" "$PROBE_JAR" "$snapshot" "$PROBE_DIR" <<'PY' || fail "$step"
import json,subprocess,sys
base,jar,raw,out=sys.argv[1:]; r=json.loads(raw)['run']; ts={t['id']:t for t in r['tickets']}
def fetch(path):
 return json.loads(subprocess.check_output(['curl','-fsS','-b',jar,base+path]))
def events(session): return fetch('/api/sessions/'+session).get('events',[])
for tid,expected in [("01-code","worker"),("02-design","worker-design")]:
 sid=ts[tid].get('workerSession'); assert sid, (tid,ts[tid])
 session=fetch('/api/sessions/'+sid).get('session',{})
 assert session.get('runWorker') is True and (expected != 'worker-design' or session.get('designSession') is True),(tid,session)
 reviewers=ts[tid].get('reviewerSessions',{}); required={'reviewer-spec','reviewer-standards'} | ({'reviewer-design'} if tid=='02-design' else set())
 assert set(reviewers)==required,(tid,reviewers)
 for role,reviewer_id in reviewers.items():
  reviewer=fetch('/api/sessions/'+reviewer_id).get('session',{})
  assert reviewer.get('reviewer')==role,(role,reviewer)
 if tid=='02-design':
  ev=events(sid); text='\n'.join(str(e.get('text','')) for e in ev)
  assert 'agent-browser' in text and ('snapshot' in text.lower() or 'designer' in text.lower()), text[-3000:]
  calls=[]
  for e in ev:
   if e.get('kind') in ('tool_call_started','tool_call_updated'):
    try: calls.append(json.loads(e.get('text','{}')))
    except Exception: pass
  completed=' '.join(str(c.get('input',''))+' '+str(c.get('output','')) for c in calls if c.get('status')=='completed')
  assert 'snapshot' in completed.lower(),completed[-3000:]
  assert any(c.get("status")=="completed" and "screenshot" in str(c.get("input","")).lower() and (".jpg" in str(c.get("input","")).lower() or ".jpeg" in str(c.get("input","")).lower()) for c in calls),calls
  assert '390' in completed and ('.jpg' in completed.lower() or '.jpeg' in completed.lower()),completed[-3000:]
  assert ('1280' in completed or 'desktop' in completed.lower()) and ('.jpg' in completed.lower() or '.jpeg' in completed.lower()),completed[-3000:]
  assert 'AGENT_BROWSER_SCREENSHOT_DIR' in text or 'agent-browser' in text
print('PASS - code Worker, worker-design Designer, injected skill and phone/desktop screenshot commands')
PY

echo "checking no leftover browser or preview process" >&2
python3 - "$PROBE_BASE" "$PROBE_JAR" "$PROBE_DIR/state" "$snapshot" <<'PY' || fail "leftover browser or preview process check"
import hashlib,json,os,re,subprocess,sys
base,jar,state,raw=sys.argv[1:]
run=json.loads(raw)['run']; sessions=set()
for ticket in run.get('tickets',[]):
    if ticket.get('workerSession'): sessions.add(ticket['workerSession'])
    sessions.update(ticket.get('reviewerSessions',{}).values())
ports=set()
for sid in sessions:
    data=json.loads(subprocess.check_output(['curl','-fsS','-b',jar,base+'/api/sessions/'+sid]))
    port=data.get('session',{}).get('previewPort')
    if port: ports.add(int(port))
namespace=hashlib.sha256(os.path.normpath(state).encode()).digest()[:8].hex()
left=[]
for entry in os.listdir('/proc'):
    if not entry.isdigit(): continue
    try:
        with open('/proc/'+entry+'/environ','rb') as f: env=f.read().split(bytes([0]))
        with open('/proc/'+entry+'/cmdline','rb') as f: cmd=f.read().replace(bytes([0]),b' ').decode(errors='replace').lower()
    except (OSError,ProcessLookupError): continue
    values={item.split(b'=',1)[0]:item.split(b'=',1)[1] for item in env if b'=' in item}
    ns=values.get(b'AGENT_BROWSER_NAMESPACE',b'').decode(errors='ignore')
    preview_session=values.get(b'EL_PREVIEW_SESSION',b'').decode(errors='ignore')
    if ns==namespace and re.search(r'agent-browser|chrom(e|ium)',cmd): left.append((entry,'browser',cmd[:180]))
    if preview_session in sessions and re.search(r'preview-server|go run',cmd): left.append((entry,'preview',cmd[:180]))
listeners=set()
for table in ('/proc/net/tcp','/proc/net/tcp6'):
    try:
        with open(table) as f:
            for row in f.readlines()[1:]:
                fields=row.split()
                if len(fields)>3 and fields[3]=='0A': listeners.add(int(fields[1].rsplit(':',1)[1],16))
    except OSError: pass
bound=ports & listeners
assert not left and not bound, {'processes':left,'previewPortsStillListening':sorted(bound)}
print('PASS - isolated browser namespace and assigned preview ports have no surviving processes/listeners')
PY
printf 'PASS - Run %s merged after worker/reviewer chain and left no browser or preview process\n' "$runid"
