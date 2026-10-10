#!/usr/bin/env bash
# Run the el-web Daemon for development on a fixed port (7941).
# Prod is the systemd user unit emberloop-web on 7940.
# Dev state lives in its own directory so it never touches prod Sessions.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root/emberloop-web"
exec go run ./cmd/emberloop-web -addr 127.0.0.1:7941 \
  -state-dir "${EL_WEB_DEV_STATE:-$HOME/.local/share/pig-dev}" "$@"
