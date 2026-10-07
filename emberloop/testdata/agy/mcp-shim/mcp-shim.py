import json, os, sys, time
# Log beside the script by default; PROBE_LOG overrides. Never die on logging.
_logpath = os.environ.get("PROBE_LOG", os.path.join(os.path.dirname(os.path.abspath(__file__)), "shim.log"))
try:
    log = open(_logpath, 'a')
except OSError:
    import io as _io
    log = _io.StringIO()
SLOW_SECS = float(os.environ.get("PROBE_SLOW_SECS", "5"))
def send(o):
    s = json.dumps(o)
    sys.stdout.write(s+"\n"); sys.stdout.flush()
    try:
        log.write('SENT: '+s[:200]+'\n'); log.flush()
    except Exception:
        pass
TOOLS = [
  {"name":"probe_echo","description":"Echo secret env","inputSchema":{"type":"object","properties":{}}},
  {"name":"probe_slow","description":"Sleep PROBE_SLOW_SECS seconds then return DONE","inputSchema":{"type":"object","properties":{}}}
]
for line in sys.stdin:
    line=line.strip()
    if not line: continue
    log.write('GOT: '+line[:300]+'\n'); log.flush()
    try: msg=json.loads(line)
    except Exception: continue
    m=msg.get("method"); i=msg.get("id"); p=msg.get("params",{})
    if m=="server/discover":
        send({"jsonrpc":"2.0","id":i,"result":{"serverInfo":{"name":"probe","version":"1"},"protocolVersion":"2026-07-28","capabilities":{"tools":{}},"tools":TOOLS}})
    elif m=="initialize":
        send({"jsonrpc":"2.0","id":i,"result":{"protocolVersion":"2026-07-28","capabilities":{"tools":{}},"serverInfo":{"name":"probe","version":"1"}}})
    elif m=="tools/list":
        send({"jsonrpc":"2.0","id":i,"result":{"tools":TOOLS}})
    elif m=="tools/call":
        name = p.get("name","")
        if name=="probe_slow":
            time.sleep(SLOW_SECS)
            send({"jsonrpc":"2.0","id":i,"result":{"content":[{"type":"text","text":"SLOW-DONE"}]}})
        else:
            send({"jsonrpc":"2.0","id":i,"result":{"content":[{"type":"text","text":"SECRET-IS-"+os.environ.get("PROBE_SECRET","MISSING")}]}})
    elif i is not None and m:
        send({"jsonrpc":"2.0","id":i,"result":{}})
