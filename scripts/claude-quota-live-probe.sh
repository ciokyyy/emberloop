#!/usr/bin/env bash
# Read-only live probe for the host Claude OAuth quota poller.
#
# Requires EL_LIVE_CLAUDE_QUOTA=1 to make an isolated el-web instance perform
# one quota refresh against Anthropic's OAuth usage endpoint. It reads
# ~/.claude/.credentials.json, sends no prompt, and never refreshes or writes
# that login. Only the normalized plan, windows, percentages and reset times
# are printed. The local daemon and quota cache live in a temporary directory.
#
# Prerequisites: go, curl, python3, network access and a readable Claude Code
# host login. This live probe is manual and is never part of `.emberloop/check`.
#
# Usage: EL_LIVE_CLAUDE_QUOTA=1 scripts/claude-quota-live-probe.sh
set -euo pipefail

if [[ "${EL_LIVE_CLAUDE_QUOTA:-}" != "1" ]]; then
  echo "SKIP - set EL_LIVE_CLAUDE_QUOTA=1 to make one read-only Claude quota request" >&2
  exit 2
fi

source "$(dirname "${BASH_SOURCE[0]}")/quota-live-probe-lib.sh"
quota_live_probe_run claude Claude "$HOME/.claude/.credentials.json"
