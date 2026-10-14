#!/usr/bin/env bash
# Read-only live probe for the pig openai-codex (ChatGPT Plus) quota poller.
#
# Requires EL_LIVE_CHATGPT_QUOTA=1 to make an isolated el-web instance
# perform one quota refresh against ChatGPT's wham/usage endpoint. It reads
# pig's existing auth.json through the poller's read-only auth store, never
# refreshes or writes that login, and prints only the normalized plan, windows,
# percentages and reset times. The local daemon and quota cache live in a
# temporary directory.
#
# Prerequisites: go, curl, python3, network access and a readable pig auth.json
# containing an openai-codex login. This live probe is manual and is never part
# of `.emberloop/check`.
#
# Usage: EL_LIVE_CHATGPT_QUOTA=1 scripts/chatgpt-quota-live-probe.sh
set -euo pipefail

if [[ "${EL_LIVE_CHATGPT_QUOTA:-}" != "1" ]]; then
  echo "SKIP - set EL_LIVE_CHATGPT_QUOTA=1 to make one read-only ChatGPT quota request" >&2
  exit 2
fi

source "$(dirname "${BASH_SOURCE[0]}")/quota-live-probe-lib.sh"
quota_live_probe_run openai-codex 'ChatGPT Plus' "$(quota_live_pig_auth_path)"
