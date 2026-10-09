#!/usr/bin/env bash
# el installer entry point. Checks for Go, then runs the installer TUI
# in ./installer. Every flag passes through: see installer/main.go.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "install.sh: Go 1.26 or newer is required (https://go.dev/dl/)." >&2
  exit 1
fi

cd "$root/installer"
exec go run . --repo "$root" "$@"
