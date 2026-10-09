#!/usr/bin/env bash
# Start a throwaway Daemon for the e2e suite: fresh state dir, fake backend,
# and two fixture folders under /tmp/el-e2e. Usage: serve.sh <port>.
set -euo pipefail
port="${1:?port required}"
root=/tmp/el-e2e
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

rm -rf "$root"
mkdir -p "$root/state" "$root/repo" "$root/add-me"
git -C "$root/repo" init -q -b main
printf 'one\n' >"$root/repo/a.txt"
git -C "$root/repo" add .
git -C "$root/repo" -c user.name=e2e -c user.email=e2e@example.test commit -q -m "first commit"
printf 'two\n' >"$root/repo/a.txt"
printf 'new\n' >"$root/repo/b.txt"

# Never touch the real settings file: the suite saves settings.
export EL_SETTINGS="$root/settings.json"

cd "$here/../.."
go build -o "$root/emberloop-web" ./cmd/emberloop-web
exec "$root/emberloop-web" -addr "127.0.0.1:$port" -state-dir "$root/state" -restart-check 0
