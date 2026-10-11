#!/usr/bin/env bash
# Probe for web-v1 ticket 32 (live probes): call the real Jev decision
# endpoint through the Jev client and check every asked question is answered.
#
# What it does, step by step:
#  1. Requires EL_LIVE_JEV=1 (else it exits 2 with a skip message, like the
#     Go live test it drives).
#  2. Runs TestLiveJevDecider in emberloop-web: one durable question plus
#     kind and scope through memory.JevDecider, asserting every asked
#     question is answered with in-range values.
#
# Verdict (manual): PASS when the test passes; FAIL otherwise. Each call
# bills like any other Jev call. Never runs in the automated check.
#
# Prerequisites: go on PATH, network access, and either JEV_API_KEY for the
# hosted endpoint or JEV_BASE_URL for a local server (a localhost URL may
# stay keyless). JEV_MODEL overrides the default model.
#
# Usage: scripts/jev-probe.sh
#   EL_LIVE_JEV=1 JEV_API_KEY=... scripts/jev-probe.sh
#   EL_LIVE_JEV=1 JEV_BASE_URL=http://localhost:8000/v1/systemone scripts/jev-probe.sh
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"

[ "${EL_LIVE_JEV:-}" != "" ] || { echo "SKIP - set EL_LIVE_JEV=1 with JEV_API_KEY for the real-endpoint probe (or JEV_BASE_URL at a local server)"; exit 2; }
command -v go >/dev/null || { echo "FAIL - need go on PATH"; exit 1; }

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "calling the live Jev endpoint (model=${JEV_MODEL:-jev-latest})..." >&2
if go -C "$root/emberloop-web" test -count=1 -run TestLiveJevDecider -v .; then
  echo "PASS - live Jev endpoint answered every question"
else
  echo "FAIL - live Jev call failed"
  exit 1
fi
