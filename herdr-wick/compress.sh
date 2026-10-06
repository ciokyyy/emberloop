#!/usr/bin/env bash
# herdr-wick compress action: signal the supervisor for the focused pane.
#
# The key binding invokes this action; herdr hands it the focused pane in
# HERDR_PLUGIN_CONTEXT_JSON. It execs `wick-pig request-compress` for that
# pane, whatever the harness: at a tool boundary the pane compresses at
# once, during a running tool call the request queues (the header shows
# `compress queued`) and a second press forces it. WICK_PIG_BIN overrides
# the binary lookup, which is what the signal test uses.
set -u

pane="$(python3 -c 'import json,os; print(json.loads(os.environ.get("HERDR_PLUGIN_CONTEXT_JSON", "{}")).get("focused_pane_id", ""))' 2>/dev/null)"
if [ -z "$pane" ]; then
  echo "wick compress: no focused pane in the plugin context" >&2
  exit 1
fi
bin="${WICK_PIG_BIN:-wick-pig}"
exec "$bin" request-compress "$pane"
